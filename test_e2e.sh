#!/bin/bash
set -e

# End-to-end CLI tests for vlt
# Tests CLI-specific behavior: flags, output formatting, error messages
# Core operations are covered by Go integration tests (make test-go-integration)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=dev-token
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

log() { echo -e "${YELLOW}==>${NC} $1"; }
pass() { echo -e "${GREEN}✓${NC} $1"; TESTS_PASSED=$((TESTS_PASSED + 1)); TESTS_RUN=$((TESTS_RUN + 1)); }
fail() { echo -e "${RED}✗${NC} $1"; TESTS_FAILED=$((TESTS_FAILED + 1)); TESTS_RUN=$((TESTS_RUN + 1)); }

check_vault() {
    if ! curl -s "$VAULT_ADDR/v1/sys/health" > /dev/null 2>&1; then
        echo -e "${RED}Error: Vault is not running at $VAULT_ADDR${NC}"
        echo "Start it with: docker compose up -d"
        exit 1
    fi
}

cleanup() { ./vlt rm -r secret/e2e 2>/dev/null || true; }

echo ""
echo "========================================"
echo "  vlt End-to-End CLI Tests"
echo "========================================"
echo ""

# Build and check
go build -o vlt . 2>/dev/null || { echo "Build failed"; exit 1; }
check_vault
cleanup

# =============================================================================
# CLI FLAGS AND OPTIONS
# =============================================================================
log "Testing CLI flags and options..."

# ls -l shows metadata
./vlt add secret/e2e/ls-test "value" 2>/dev/null
output=$(./vlt ls -l secret/e2e 2>/dev/null)
if [[ "$output" == *"ls-test"* ]] && [[ "$output" == *"v"* ]]; then
    pass "ls -l: shows version metadata"
else
    fail "ls -l: metadata (got: $output)"
fi

# rm -r required for directories
./vlt add secret/e2e/rm-dir/a "a" 2>/dev/null
./vlt add secret/e2e/rm-dir/b "b" 2>/dev/null
if ./vlt rm secret/e2e/rm-dir 2>/dev/null; then
    fail "rm: should require -r for directory"
else
    pass "rm: requires -r for directory"
fi
./vlt rm -r secret/e2e/rm-dir 2>/dev/null

# copy -r for recursive
./vlt add secret/e2e/cp-src/a "a" 2>/dev/null
./vlt add secret/e2e/cp-src/b "b" 2>/dev/null
if ./vlt copy -r secret/e2e/cp-src secret/e2e/cp-dst 2>/dev/null; then
    if ./vlt ls secret/e2e/cp-dst 2>/dev/null | grep -q "a"; then
        pass "copy -r: recursive copy"
    else
        fail "copy -r: missing files"
    fi
else
    fail "copy -r: command failed"
fi

# import --dry-run
cat > "$TMPDIR/import.yaml" << 'EOF'
admin:
  password: secret
database:
  url: postgres://localhost
EOF
output=$(./vlt import --dry-run "$TMPDIR/import.yaml" secret/e2e/import 2>/dev/null)
if [[ "$output" == *"dry-run"* ]]; then
    pass "import --dry-run: shows preview"
else
    fail "import --dry-run (got: $output)"
fi

# import --append-name
cat > "$TMPDIR/app-secrets.yaml" << 'EOF'
api_key: secret-key
EOF
if ./vlt import --append-name "$TMPDIR/app-secrets.yaml" secret/e2e/named 2>/dev/null; then
    if ./vlt ls secret/e2e/named/app 2>/dev/null | grep -q "api_key"; then
        pass "import --append-name: uses filename"
    else
        fail "import --append-name: path wrong"
    fi
else
    fail "import --append-name: failed"
fi

# import --update-counterpart (counterpart filename is derived: app-secrets.yaml -> app.yaml)
cat > "$TMPDIR/app-secrets.yaml" << 'EOF'
admin:
  password: super-secret
EOF
cat > "$TMPDIR/app.yaml" << 'EOF'
admin:
  password: placeholder
EOF
if ./vlt import --update-counterpart "$TMPDIR/app-secrets.yaml" secret/e2e/counterpart 2>/dev/null; then
    if grep -q "ref+vault://secret/e2e/counterpart/admin.password#value" "$TMPDIR/app.yaml"; then
        pass "import --update-counterpart: updates file"
    else
        fail "import --update-counterpart: not updated"
    fi
else
    fail "import --update-counterpart: failed"
fi

# export -o
./vlt add secret/e2e/export/key "value" 2>/dev/null
if ./vlt export secret/e2e/export -o "$TMPDIR/exported.yaml" 2>/dev/null; then
    if [[ -f "$TMPDIR/exported.yaml" ]] && grep -q "key" "$TMPDIR/exported.yaml"; then
        pass "export -o: writes to file"
    else
        fail "export -o: file content wrong"
    fi
else
    fail "export -o: failed"
fi

# snapshot -o and restore --dry-run
./vlt add secret/e2e/snap/key "original" 2>/dev/null
./vlt snapshot secret/e2e/snap -o "$TMPDIR/snap.yaml" 2>/dev/null
./vlt update secret/e2e/snap/key "modified" 2>/dev/null
output=$(./vlt restore --dry-run "$TMPDIR/snap.yaml" secret/e2e/snap 2>&1)
if [[ "$output" == *"dry-run"* ]] && [[ "$output" == *"Updated"* ]]; then
    pass "restore --dry-run: shows preview"
else
    fail "restore --dry-run (got: $output)"
fi

# restore --no-delete
./vlt add secret/e2e/snap/extra "extra" 2>/dev/null
if ./vlt restore --no-delete "$TMPDIR/snap.yaml" secret/e2e/snap 2>/dev/null; then
    if ./vlt get secret/e2e/snap/extra value 2>/dev/null | grep -q "extra"; then
        pass "restore --no-delete: preserves extras"
    else
        fail "restore --no-delete: deleted extra"
    fi
else
    fail "restore --no-delete: failed"
fi

# restore --verify
./vlt add secret/e2e/verify/test "v1" 2>/dev/null
./vlt snapshot secret/e2e/verify -o "$TMPDIR/verify.yaml" 2>/dev/null
./vlt update secret/e2e/verify/test "v2" 2>/dev/null
./vlt update secret/e2e/verify/test "v3" 2>/dev/null
output=$(./vlt restore --verify "$TMPDIR/verify.yaml" secret/e2e/verify 2>&1)
if [[ "$output" == *"Skipped"* ]]; then
    pass "restore --verify: skips version mismatch"
else
    fail "restore --verify (got: $output)"
fi

# diff --quiet
./vlt add secret/e2e/diff-a "same" 2>/dev/null
./vlt add secret/e2e/diff-b "same" 2>/dev/null
if ./vlt diff --quiet secret/e2e/diff-a secret/e2e/diff-b 2>/dev/null; then
    pass "diff --quiet: exit 0 for identical"
else
    fail "diff --quiet: wrong exit code"
fi

# diff --summary
./vlt add secret/e2e/diff-c "different" 2>/dev/null
output=$(./vlt diff --summary secret/e2e/diff-a secret/e2e/diff-c 2>&1) || true
if [[ "$output" == *"Changed:"* ]]; then
    pass "diff --summary: shows counts"
else
    fail "diff --summary (got: $output)"
fi

# diff --show-values
output=$(./vlt diff --show-values secret/e2e/diff-a secret/e2e/diff-c 2>&1) || true
if [[ "$output" == *"same"* ]] && [[ "$output" == *"different"* ]]; then
    pass "diff --show-values: shows actual values"
else
    fail "diff --show-values (got: $output)"
fi

# history -v (verbose)
./vlt add secret/e2e/hist "v1" 2>/dev/null
./vlt update secret/e2e/hist "v2" 2>/dev/null
output=$(./vlt history secret/e2e/hist -v 2>&1)
if [[ "$output" == *"v2"* ]] && [[ "$output" == *"v1"* ]]; then
    pass "history -v: shows versions"
else
    fail "history -v (got: $output)"
fi

# history -n (limit)
./vlt update secret/e2e/hist "v3" 2>/dev/null
output=$(./vlt history secret/e2e/hist -n 1 2>&1)
if [[ "$output" == *"more entries"* ]] || [[ $(echo "$output" | grep -c "v[0-9]") -le 2 ]]; then
    pass "history -n: limits output"
else
    fail "history -n (got: $output)"
fi

# tree -l
./vlt add secret/e2e/tree/config "val" 2>/dev/null
./vlt add secret/e2e/tree/db/host "host" 2>/dev/null
output=$(./vlt tree secret/e2e/tree -l 2>&1)
if [[ "$output" == *"v1"* ]] && [[ "$output" == *"├──"* ]]; then
    pass "tree -l: shows metadata"
else
    fail "tree -l (got: $output)"
fi

# =============================================================================
# OUTPUT FORMATTING
# =============================================================================
log "Testing output formatting..."

# tree structure
output=$(./vlt tree secret/e2e/tree 2>&1)
if [[ "$output" == *"tree/"* ]] && [[ "$output" == *"├──"* ]] && [[ "$output" == *"└──"* ]]; then
    pass "tree: proper structure with box chars"
else
    fail "tree: structure (got: $output)"
fi

# tree summary
if [[ "$output" == *"directories"* ]] && [[ "$output" == *"secrets"* ]]; then
    pass "tree: shows summary line"
else
    fail "tree: summary (got: $output)"
fi

# diff output format
./vlt add secret/e2e/fmt-a/key1 "val1" 2>/dev/null
./vlt add secret/e2e/fmt-a/key2 "val2" 2>/dev/null
./vlt add secret/e2e/fmt-b/key1 "val1" 2>/dev/null
./vlt add secret/e2e/fmt-b/key2 "different" 2>/dev/null
./vlt add secret/e2e/fmt-b/key3 "val3" 2>/dev/null
output=$(./vlt diff secret/e2e/fmt-a secret/e2e/fmt-b 2>&1) || true
if [[ "$output" == *"Only in"* ]] && [[ "$output" == *"Changed"* ]]; then
    pass "diff: shows only-in and changed sections"
else
    fail "diff: format (got: $output)"
fi

# duplicates output
./vlt add secret/e2e/dup/a "same-value" 2>/dev/null
./vlt add secret/e2e/dup/b "same-value" 2>/dev/null
output=$(./vlt duplicates secret/e2e/dup 2>&1)
if [[ "$output" == *"Duplicate"* ]] && [[ "$output" == *"a"* ]] && [[ "$output" == *"b"* ]]; then
    pass "duplicates: shows duplicate paths"
else
    fail "duplicates (got: $output)"
fi

# get YAML output
./vlt add secret/e2e/yaml/nested/deep "value" 2>/dev/null
output=$(./vlt get secret/e2e/yaml 2>/dev/null)
if [[ "$output" == *"nested:"* ]] && [[ "$output" == *"deep:"* ]]; then
    pass "get: outputs nested YAML"
else
    fail "get: YAML format (got: $output)"
fi

# =============================================================================
# ERROR MESSAGES
# =============================================================================
log "Testing error messages..."

# add to existing
./vlt add secret/e2e/exists "original" 2>/dev/null
output=$(./vlt add secret/e2e/exists "new" 2>&1) || true
if [[ "$output" == *"already exists"* ]]; then
    pass "add: error for existing secret"
else
    fail "add: error message (got: $output)"
fi

# update non-existent
output=$(./vlt update secret/e2e/nonexistent "value" 2>&1) || true
if [[ "$output" == *"not found"* ]]; then
    pass "update: error for non-existent"
else
    fail "update: error message (got: $output)"
fi

# copy to existing
./vlt add secret/e2e/copy-dst "exists" 2>/dev/null
output=$(./vlt copy secret/e2e/exists secret/e2e/copy-dst 2>&1) || true
if [[ "$output" == *"already exists"* ]]; then
    pass "copy: error for existing dest"
else
    fail "copy: error message (got: $output)"
fi

# diff @prev on v1
./vlt add secret/e2e/v1only "value" 2>/dev/null
output=$(./vlt diff secret/e2e/v1only@prev secret/e2e/v1only 2>&1) || true
if [[ "$output" == *"no previous"* ]] || [[ "$output" == *"version 1"* ]]; then
    pass "diff @prev: error for v1 only"
else
    fail "diff @prev: error message (got: $output)"
fi

# =============================================================================
# EDIT COMMAND (requires editor)
# =============================================================================
log "Testing edit command..."

./vlt add secret/e2e/edit/config "original" 2>/dev/null

# Edit with fake editor that modifies value
cat > "$TMPDIR/editor.sh" << 'EOF'
#!/bin/bash
sed -i.bak 's/original/edited/' "$1"
EOF
chmod +x "$TMPDIR/editor.sh"

if EDITOR="$TMPDIR/editor.sh" ./vlt edit secret/e2e/edit/config 2>/dev/null; then
    output=$(./vlt get secret/e2e/edit/config value 2>/dev/null)
    if [[ "$output" == "edited" ]]; then
        pass "edit: updates secret"
    else
        fail "edit: value not updated (got: $output)"
    fi
else
    fail "edit: command failed"
fi

# Edit with no changes
cat > "$TMPDIR/noop-editor.sh" << 'EOF'
#!/bin/bash
exit 0
EOF
chmod +x "$TMPDIR/noop-editor.sh"
output=$(EDITOR="$TMPDIR/noop-editor.sh" ./vlt edit secret/e2e/edit/config 2>&1)
if [[ "$output" == *"no changes"* ]]; then
    pass "edit: detects no changes"
else
    fail "edit: no changes detection (got: $output)"
fi

# Recursive edit with deletion
./vlt add secret/e2e/edit-dir/a "val-a" 2>/dev/null
./vlt add secret/e2e/edit-dir/b "val-b" 2>/dev/null
cat > "$TMPDIR/delete-editor.sh" << 'EOF'
#!/bin/bash
grep -v "^b:" "$1" > "$1.tmp" && mv "$1.tmp" "$1"
EOF
chmod +x "$TMPDIR/delete-editor.sh"

if EDITOR="$TMPDIR/delete-editor.sh" ./vlt edit secret/e2e/edit-dir 2>/dev/null; then
    if ./vlt get secret/e2e/edit-dir/b value 2>/dev/null; then
        fail "edit: delete not applied"
    else
        pass "edit: recursive delete"
    fi
else
    fail "edit: recursive command failed"
fi

# =============================================================================
# VERSION AND DIFF FEATURES
# =============================================================================
log "Testing version features..."

# diff between versions
./vlt add secret/e2e/versioned "version1" 2>/dev/null
./vlt update secret/e2e/versioned "version2" 2>/dev/null
output=$(./vlt diff --show-values secret/e2e/versioned@1 secret/e2e/versioned@2 2>&1) || true
if [[ "$output" == *"version1"* ]] && [[ "$output" == *"version2"* ]]; then
    pass "diff @version: compares versions"
else
    fail "diff @version (got: $output)"
fi

# diff @prev
output=$(./vlt diff --show-values secret/e2e/versioned@prev secret/e2e/versioned 2>&1) || true
if [[ "$output" == *"version1"* ]] && [[ "$output" == *"version2"* ]]; then
    pass "diff @prev: alias works"
else
    fail "diff @prev (got: $output)"
fi

# directory @prev
./vlt add secret/e2e/dir-ver/a "a-v1" 2>/dev/null
./vlt add secret/e2e/dir-ver/b "b-v1" 2>/dev/null
./vlt update secret/e2e/dir-ver/a "a-v2" 2>/dev/null
./vlt update secret/e2e/dir-ver/b "b-v2" 2>/dev/null
output=$(./vlt diff --show-values secret/e2e/dir-ver@prev secret/e2e/dir-ver 2>&1) || true
if [[ "$output" == *"a-v1"* ]] && [[ "$output" == *"a-v2"* ]]; then
    pass "diff directory @prev: works"
else
    fail "diff directory @prev (got: $output)"
fi

# directory @-N timeline
output=$(./vlt diff secret/e2e/dir-ver@-1 secret/e2e/dir-ver 2>&1) || true
if [[ "$output" == *"Changed"* ]]; then
    pass "diff @-N: timeline works"
else
    fail "diff @-N (got: $output)"
fi

# =============================================================================
# EDGE CASES
# =============================================================================
log "Testing edge cases..."

# Trailing slash handling
./vlt add secret/e2e/trailing "value" 2>/dev/null
if ./vlt get secret/e2e/trailing/ 2>/dev/null | grep -q "value"; then
    pass "path: handles trailing slash"
else
    fail "path: trailing slash"
fi

# Unicode values
if ./vlt add secret/e2e/unicode "Hello 世界 🔐" 2>/dev/null; then
    output=$(./vlt get secret/e2e/unicode value 2>/dev/null)
    if [[ "$output" == *"世界"* ]] && [[ "$output" == *"🔐"* ]]; then
        pass "value: unicode preserved"
    else
        fail "value: unicode"
    fi
else
    fail "value: unicode add"
fi

# Multiline values
if printf "line1\nline2\nline3" | ./vlt add secret/e2e/multiline - 2>/dev/null; then
    output=$(./vlt get secret/e2e/multiline value 2>/dev/null)
    if [[ "$output" == *"line1"* ]] && [[ "$output" == *"line2"* ]]; then
        pass "value: multiline preserved"
    else
        fail "value: multiline"
    fi
else
    fail "value: multiline add"
fi

# YAML special characters
if ./vlt add secret/e2e/yaml-chars "key: value, with: colons" 2>/dev/null; then
    output=$(./vlt get secret/e2e/yaml-chars value 2>/dev/null)
    if [[ "$output" == "key: value, with: colons" ]]; then
        pass "value: YAML special chars preserved"
    else
        fail "value: YAML chars (got: $output)"
    fi
else
    fail "value: YAML chars add"
fi

# JSON value
if ./vlt add secret/e2e/json '{"key": "value", "nested": {"a": 1}}' 2>/dev/null; then
    output=$(./vlt get secret/e2e/json value 2>/dev/null)
    if [[ "$output" == *'"key"'* ]]; then
        pass "value: JSON content preserved"
    else
        fail "value: JSON"
    fi
else
    fail "value: JSON add"
fi

# Deeply nested paths
if ./vlt add secret/e2e/a/b/c/d/e/f/deep "value" 2>/dev/null; then
    output=$(./vlt get secret/e2e/a/b/c/d/e/f/deep value 2>/dev/null)
    if [[ "$output" == "value" ]]; then
        pass "path: deeply nested (6 levels)"
    else
        fail "path: deep nesting"
    fi
else
    fail "path: deep nesting add"
fi

# Export/import round-trip
./vlt add secret/e2e/roundtrip/key1 "value1" 2>/dev/null
./vlt add secret/e2e/roundtrip/key2 "value2" 2>/dev/null
./vlt export secret/e2e/roundtrip -o "$TMPDIR/roundtrip.yaml" 2>/dev/null
./vlt rm -r secret/e2e/roundtrip 2>/dev/null
./vlt import "$TMPDIR/roundtrip.yaml" secret/e2e/roundtrip 2>/dev/null
v1=$(./vlt get secret/e2e/roundtrip/key1 value 2>/dev/null)
v2=$(./vlt get secret/e2e/roundtrip/key2 value 2>/dev/null)
if [[ "$v1" == "value1" ]] && [[ "$v2" == "value2" ]]; then
    pass "export/import: round-trip safe"
else
    fail "export/import: round-trip"
fi

# Snapshot/restore round-trip with special chars
./vlt add secret/e2e/snap-special/unicode "Hello 世界" 2>/dev/null
./vlt snapshot secret/e2e/snap-special -o "$TMPDIR/special.yaml" 2>/dev/null
./vlt rm -r secret/e2e/snap-special 2>/dev/null
./vlt restore "$TMPDIR/special.yaml" secret/e2e/snap-special 2>/dev/null
output=$(./vlt get secret/e2e/snap-special/unicode value 2>/dev/null)
if [[ "$output" == *"世界"* ]]; then
    pass "snapshot/restore: special chars preserved"
else
    fail "snapshot/restore: special chars"
fi

# Diff with local file
cat > "$TMPDIR/local.yaml" << 'EOF'
key1: value1
key2: value2
EOF
./vlt add secret/e2e/local-diff/key1 "value1" 2>/dev/null
./vlt add secret/e2e/local-diff/key2 "value2" 2>/dev/null
if ./vlt diff "$TMPDIR/local.yaml" secret/e2e/local-diff 2>/dev/null; then
    pass "diff: local file comparison"
else
    fail "diff: local file"
fi

# =============================================================================
# WORKFLOW SCENARIOS
# =============================================================================
log "Testing workflow scenarios..."

# Disaster recovery
./vlt add secret/e2e/dr/config "config" 2>/dev/null
./vlt add secret/e2e/dr/db/password "secret" 2>/dev/null
./vlt snapshot secret/e2e/dr -o "$TMPDIR/dr-backup.yaml" 2>/dev/null
./vlt rm -r secret/e2e/dr 2>/dev/null
if ./vlt restore "$TMPDIR/dr-backup.yaml" secret/e2e/dr 2>/dev/null; then
    config=$(./vlt get secret/e2e/dr/config value 2>/dev/null)
    dbpass=$(./vlt get secret/e2e/dr/db/password value 2>/dev/null)
    if [[ "$config" == "config" ]] && [[ "$dbpass" == "secret" ]]; then
        pass "workflow: disaster recovery"
    else
        fail "workflow: DR incomplete"
    fi
else
    fail "workflow: DR restore failed"
fi

# Environment promotion
./vlt add secret/e2e/staging/app/key "staging-key" 2>/dev/null
./vlt snapshot secret/e2e/staging -o "$TMPDIR/staging.yaml" 2>/dev/null
./vlt rm -r secret/e2e/prod 2>/dev/null || true
if ./vlt restore "$TMPDIR/staging.yaml" secret/e2e/prod 2>/dev/null; then
    prod_key=$(./vlt get secret/e2e/prod/app/key value 2>/dev/null)
    if [[ "$prod_key" == "staging-key" ]]; then
        pass "workflow: environment promotion"
    else
        fail "workflow: promotion failed"
    fi
else
    fail "workflow: promotion restore failed"
fi

# Rollback
./vlt add secret/e2e/rollback/config "v1" 2>/dev/null
./vlt snapshot secret/e2e/rollback -o "$TMPDIR/v1.yaml" 2>/dev/null
./vlt update secret/e2e/rollback/config "v2-broken" 2>/dev/null
./vlt add secret/e2e/rollback/bad "oops" 2>/dev/null
if ./vlt restore "$TMPDIR/v1.yaml" secret/e2e/rollback 2>/dev/null; then
    config=$(./vlt get secret/e2e/rollback/config value 2>/dev/null)
    bad=$(./vlt get secret/e2e/rollback/bad value 2>/dev/null) || bad=""
    if [[ "$config" == "v1" ]] && [[ -z "$bad" ]]; then
        pass "workflow: rollback to snapshot"
    else
        fail "workflow: rollback incomplete"
    fi
else
    fail "workflow: rollback failed"
fi

# =============================================================================
# CLEANUP AND SUMMARY
# =============================================================================
log "Cleaning up..."
cleanup

echo ""
echo "========================================"
echo "  Test Summary"
echo "========================================"
echo ""
echo -e "Tests run:    $TESTS_RUN"
echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
echo ""

if [[ $TESTS_FAILED -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
