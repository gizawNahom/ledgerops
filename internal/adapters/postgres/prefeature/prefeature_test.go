package prefeature

import (
	"crypto/sha256"
	"encoding/hex"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// frozenChecksums pins every frozen file (sha256 of the bytes). These values
// were taken from the migration files as they stood on main at commit
// 32df54f, before 0001 was edited. If this test fails, someone edited the
// fixture: restore it, do not update the table.
var frozenChecksums = map[string]string{
	"0001_init.down.sql":                                    "301294d40bea6f87a5cac9c8f128f24b33c9ea1f01b99f110525eed1a4c57e75",
	"0001_init.up.sql":                                      "c41932236bb327b05e268bf217c29ab3afa7192ffb356afc8f15900fff3ca0f6",
	"0002_idempotency.down.sql":                             "1ee0d4d8cefa6a30b24a8ca09061b36f60b9a1294ac6fd363d14812aaf73f72e",
	"0002_idempotency.up.sql":                               "e77af20a96bfb95cbe3096375f70763c8ed2f9847823b0d52e0c84c01bc21445",
	"0003_tenants.down.sql":                                 "c326a61962354e9971f56bd6d359d5cffb3e69b0bd0ee0f54837d3b04b1824b2",
	"0003_tenants.up.sql":                                   "8a11188e28aa9a043c008923bec95eb80d1f97b6b9ccf721eee0b00032ddbe1e",
	"0004_tenant_links.down.sql":                            "df1146c0327ef6b66c875b6d264a1fc0639d45fea0537eee46f874f59a10a7d0",
	"0004_tenant_links.up.sql":                              "7144b57781e6a2c71a4a3f1d0a24eddaa99b50aa626758e4436cac2746ebe9e4",
	"0005_counterparty_aliases_and_transfer_state.down.sql": "d92a44872a32f32c95fa7e4da6c282d7f5f76bf97174234b791d55185aebfc04",
	"0005_counterparty_aliases_and_transfer_state.up.sql":   "4ec6eac19853057e519c317c6d2c26490255af0b443ae6a6735c9d3c4f194df8",
	"0006_transfer_state_leg_details.down.sql":              "88fc30840ad74f65faacb68a85e310832c14e519fe07697ca4742cb26ef10a8a",
	"0006_transfer_state_leg_details.up.sql":                "1a94eb05b391940877dec164f631f6d9f5a45b3e5b4ce5f1ad0304b87e0d7c5e",
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestFrozenFilesMatchTheirPinnedChecksums(t *testing.T) {
	seen := map[string]bool{}
	entries, err := fs.ReadDir(FS, Dir)
	if err != nil {
		t.Fatalf("reading the frozen set: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		seen[name] = true
		want, pinned := frozenChecksums[name]
		if !pinned {
			t.Errorf("%s is in the frozen set but not pinned: the fixture must not grow", name)
			continue
		}
		contents, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if got := sha(contents); got != want {
			t.Errorf("%s changed: sha256 %s, pinned %s. The pre-feature fixture is frozen; restore it.", name, got, want)
		}
	}
	for name := range frozenChecksums {
		if !seen[name] {
			t.Errorf("%s is pinned but missing from the frozen set", name)
		}
	}
}

// The fixture is only a fixture if it still IS the old behaviour. These two
// assertions are the Earned-Trust check on the frozen text itself.
func TestFrozenZeroOneStillCreatesTheDemoRole(t *testing.T) {
	contents, err := fs.ReadFile(FS, "0001_init.up.sql")
	if err != nil {
		t.Fatalf("reading frozen 0001: %v", err)
	}
	text := string(contents)
	if !strings.Contains(text, "CREATE ROLE ledgerops_app LOGIN PASSWORD 'app-secret'") {
		t.Error("frozen 0001 no longer creates ledgerops_app LOGIN with the demo password")
	}
	if !strings.Contains(text, "GRANT CONNECT ON DATABASE ledgerops TO ledgerops_app") {
		t.Error("frozen 0001 no longer grants CONNECT on the literal database name ledgerops")
	}
}

// liveMigrationsDir is the live (embedded) migration set, relative to this
// package directory.
const liveMigrationsDir = "../migrations"

// 0002-0006 are not edited by this feature. The live files must stay byte
// identical to the frozen ones (expand-only: applied migrations are immutable,
// ADR-017 records 0001 as the single exception).
func TestLiveZeroTwoThroughSixAreStillByteIdenticalToTheFrozenOnes(t *testing.T) {
	for name, want := range frozenChecksums {
		if strings.HasPrefix(name, "0001_") {
			continue
		}
		live, err := os.ReadFile(filepath.Join(liveMigrationsDir, name))
		if err != nil {
			t.Errorf("live %s: %v", name, err)
			continue
		}
		if got := sha(live); got != want {
			t.Errorf("live %s was edited after it shipped (sha256 %s, frozen %s): applied migrations are immutable", name, got, want)
		}
	}
}

// The edited 0001 is pinned: ADR-017 permits exactly one in-place edit.
func TestLiveZeroOneIsPinnedOnceEdited(t *testing.T) {
	live, err := os.ReadFile(filepath.Join(liveMigrationsDir, "0001_init.up.sql"))
	if err != nil {
		t.Fatalf("live 0001: %v", err)
	}
	if sha(live) == frozenChecksums["0001_init.up.sql"] {
		t.Fatal("live 0001 still equals the frozen text: the DDD-27 edit has not landed")
	}
	const editedZeroOneChecksum = "c48ba421e7229e71b357cfd46cd11eaa39622d7f60c0d618f9774ffe3bf32d62"
	if got := sha(live); got != editedZeroOneChecksum {
		t.Errorf("live 0001 changed after the one permitted edit: sha256 %s, pinned %s", got, editedZeroOneChecksum)
	}
}

// The frozen SQL carries a well-known password. It must never be linkable into
// the serving binary or the domain/application core.
func TestFrozenFixtureIsNeverImportedByTheServingPathOrTheCore(t *testing.T) {
	const forbidden = "ledgerops/internal/adapters/postgres/prefeature"
	repoRoot := filepath.Join("..", "..", "..", "..")
	for _, dir := range []string{"cmd", filepath.Join("internal", "domain"), filepath.Join("internal", "app")} {
		root := filepath.Join(repoRoot, dir)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if perr != nil {
				return perr
			}
			for _, imp := range file.Imports {
				if value, uerr := strconv.Unquote(imp.Path.Value); uerr == nil && value == forbidden {
					t.Errorf("%s imports the frozen pre-feature fixture; only scripts/smoke and the acceptance suites may", path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scanning %s: %v", root, err)
		}
	}
}
