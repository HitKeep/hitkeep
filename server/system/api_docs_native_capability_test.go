package system

import (
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"hitkeep/config"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
	"hitkeep/testutil/testdb"
)

func TestNativeAndroidCapabilityOpenAPIAllowsConfiguredRuntimeValues(t *testing.T) {
	store := testdb.Shared(t)
	defer store.Close()
	// Synthetic digest exercises opt-in configuration, not a real release identity.
	fingerprint := strings.TrimSuffix(strings.Repeat("AB:", 32), ":")
	for _, configured := range []bool{false, true} {
		name := "default"
		if configured {
			name = "configured"
		}
		t.Run(name, func(t *testing.T) {
			conf := &config.Config{PublicURL: "https://example.test"}
			if configured {
				conf.PasskeyAndroidPackageName = "test.example.release"
				conf.PasskeyAndroidSHA256Fingerprints = fingerprint
			}
			ctx := &shared.Context{Config: conf, Store: store, AuthState: shared.NewAuthStateStore()}
			status := ctx.NativeAuthStatus()
			if status == nil || status.NativeAndroidPasskeys != configured {
				t.Fatalf("expected actual runtime native_android_passkeys=%v, got %+v", configured, status)
			}
			spec := OpenAPISpecV1ForRuntime(conf.PublicURL, ctx)
			schemas := requireMap(t, requireMap(t, spec, "components"), "schemas")
			statusRef := requireMap(t, requireMap(t, requireMap(t, schemas, "SystemStatus"), "properties"), "native_auth")
			if statusRef["$ref"] != "#/components/schemas/NativeAuthStatus" {
				t.Fatalf("status no longer references native capability schema: %v", statusRef)
			}
			properties := requireMap(t, requireMap(t, schemas, "NativeAuthStatus"), "properties")
			field := requireMap(t, properties, "native_android_passkeys")
			encoded, err := json.Marshal(field)
			if err != nil {
				t.Fatal(err)
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(encoded, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(status.NativeAndroidPasskeys); err != nil {
				t.Errorf("OpenAPI rejected actual native_android_passkeys=%v: %v", status.NativeAndroidPasskeys, err)
			}
			if err := resolved.Validate("true"); err == nil {
				t.Error("capability schema must reject a string in place of a boolean")
			}
			if description, _ := field["description"].(string); description == "" {
				t.Error("capability schema must explain configured-only availability")
			}
		})
	}
}
