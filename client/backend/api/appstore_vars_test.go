package api

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
	"testing"
)

func TestExtractComposeVarRefs(t *testing.T) {
	compose := `
services:
  db:
    image: postgres
    environment:
      - POSTGRES_PASSWORD=${DB_PASS:-p}
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $DB_USER -d ${DB_NAME}"]
    labels:
      - "traefik.http.routers.db.rule=Host(${DB_HOST})"
  web:
    command: ["sh", "-c", "echo $$HOME && echo $WEB_HOST && echo ${WEB_PORT:-8080}"]
`
	refs := extractComposeVarRefs(compose)

	if _, ok := refs["DB_PASS"]; !ok {
		t.Fatalf("expected DB_PASS in refs")
	}
	if !refs["DB_PASS"].HasDefault || refs["DB_PASS"].DefaultValue != "p" {
		t.Fatalf("expected DB_PASS default p, got %+v", refs["DB_PASS"])
	}
	if _, ok := refs["DB_USER"]; !ok {
		t.Fatalf("expected DB_USER in refs")
	}
	if _, ok := refs["DB_NAME"]; !ok {
		t.Fatalf("expected DB_NAME in refs")
	}
	if _, ok := refs["DB_HOST"]; !ok {
		t.Fatalf("expected DB_HOST in refs")
	}
	if _, ok := refs["WEB_HOST"]; !ok {
		t.Fatalf("expected WEB_HOST in refs")
	}
	if _, ok := refs["WEB_PORT"]; !ok {
		t.Fatalf("expected WEB_PORT in refs")
	}
	if _, ok := refs["HOME"]; ok {
		t.Fatalf("expected HOME not to be extracted from $$HOME")
	}
}

func TestExtractComposeInterpolationKeysIncludesDollarVar(t *testing.T) {
	compose := "services:\n  a:\n    command: echo $A && echo ${B:-2}\n"
	keys := extractComposeInterpolationKeys(compose)
	if _, ok := keys["A"]; !ok {
		t.Fatalf("expected A key")
	}
	if _, ok := keys["B"]; !ok {
		t.Fatalf("expected B key")
	}
}

func TestExtractComposeInterpolationKeysIncludesRequiredVarSyntax(t *testing.T) {
	compose := `
services:
  app:
    environment:
      APP_SECRET: ${APP_SECRET:?required}
      DB_PASSWORD: ${DB_PASSWORD?required}
      OPTIONAL: ${OPTIONAL:-default}
      ALT: ${ALT:+enabled}
`
	keys := extractComposeInterpolationKeys(compose)
	for _, key := range []string{"APP_SECRET", "DB_PASSWORD", "OPTIONAL", "ALT"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("expected %s key", key)
		}
	}
}

func TestFilterDotenvByAllowedKeysKeepsRequiredVarSyntax(t *testing.T) {
	compose := "services:\n  app:\n    environment:\n      APP_SECRET: ${APP_SECRET:?required}\n"
	dotenv := "APP_SECRET=secret\nUNUSED=value\n"

	filtered := filterDotenvByAllowedKeys(dotenv, extractComposeInterpolationKeys(compose))
	if !strings.Contains(filtered, "APP_SECRET=secret") {
		t.Fatalf("expected APP_SECRET to be kept, got %q", filtered)
	}
	if strings.Contains(filtered, "UNUSED=value") {
		t.Fatalf("expected UNUSED to be filtered, got %q", filtered)
	}
}

func TestExtractComposeVarRefsMaxVars(t *testing.T) {
	var b strings.Builder
	b.WriteString("services:\n  a:\n    command: |\n      echo ")
	for i := 0; i < 1200; i++ {
		b.WriteString("${K_")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("}\n      echo ")
	}
	refs := extractComposeVarRefs(b.String())
	if len(refs) > 500 {
		t.Fatalf("expected refs size <= 500, got %d", len(refs))
	}
}

func TestExtractServiceEnvFileRefs(t *testing.T) {
	compose := `
services:
  web:
    image: nginx
    env_file:
      - .env
      - path: .env.web
        required: true
      - path: .env.optional
        required: false
  api:
    env_file: .env.api
`
	m, err := extractServiceEnvFileRefs(compose)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	web := m["web"]
	if len(web) != 3 {
		t.Fatalf("expected web env_file size 3, got %d", len(web))
	}
	api := m["api"]
	if len(api) != 1 || strings.TrimSpace(api[0].Path) != ".env.api" {
		t.Fatalf("expected api env_file .env.api, got %+v", api)
	}
}

func TestExtractComposeSecrets(t *testing.T) {
	compose := `
services:
  web:
    image: nginx
    secrets:
      - db_pass
      - source: api_key
        target: api_key.txt
secrets:
  db_pass:
    file: ./secrets/db_pass.txt
  api_key:
    external: true
`
	defs, uses, err := extractComposeSecrets(compose)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if defs["db_pass"].File == "" || defs["db_pass"].External {
		t.Fatalf("expected db_pass file secret, got %+v", defs["db_pass"])
	}
	if !defs["api_key"].External {
		t.Fatalf("expected api_key external secret, got %+v", defs["api_key"])
	}
	if len(uses) != 2 {
		t.Fatalf("expected 2 secret uses, got %d", len(uses))
	}
}

func TestRewriteComposeFixedAssetPaths(t *testing.T) {
	compose := `
services:
  web:
    image: nginx
    env_file:
      - .env
      - .env.web
    secrets:
      - db_pass
secrets:
  db_pass:
    file: ./secrets/db_pass.txt
`
	envRefs, err := extractEnvFileRefs(compose)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	envMap, err := buildFixedEnvFilePathMap(envRefs)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defs, _, err := extractComposeSecrets(compose)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	secMap := buildFixedSecretFilePathMap(defs)
	out, err := rewriteComposeFixedAssetPaths(compose, envMap, secMap)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	sum := crc32.ChecksumIEEE([]byte(".env.web"))
	expectEnv := fmt.Sprintf("%s/.env.web-%08x.env", fixedEnvFilesDir, sum)
	if !strings.Contains(out, expectEnv) {
		t.Fatalf("expected rewritten env_file %s, got:\n%s", expectEnv, out)
	}
	if !strings.Contains(out, fmt.Sprintf("%s/db_pass", fixedSecretsDir)) {
		t.Fatalf("expected rewritten secret path, got:\n%s", out)
	}
}

func TestUpsertDotenvValuesOverridesExisting(t *testing.T) {
	in := "WEB_PORT=8080\n# keep\nexport TOKEN=old\n"
	out := upsertDotenvValues(in, map[string]string{
		"WEB_PORT": "3000",
		"TOKEN":    "new",
		"DATA_DIR": "/data/app",
	})

	if !strings.Contains(out, "WEB_PORT=3000\n") {
		t.Fatalf("expected WEB_PORT override, got:\n%s", out)
	}
	if !strings.Contains(out, "export TOKEN=new\n") {
		t.Fatalf("expected export TOKEN override, got:\n%s", out)
	}
	if !strings.Contains(out, "DATA_DIR=/data/app\n") {
		t.Fatalf("expected DATA_DIR append, got:\n%s", out)
	}
	if strings.Contains(out, "WEB_PORT=8080") || strings.Contains(out, "TOKEN=old") {
		t.Fatalf("expected old values removed, got:\n%s", out)
	}
}

func TestResolveAppStoreAssetURL(t *testing.T) {
	got := resolveAppStoreAssetURL("/uploads/demo.png", "https://store.example.com/")
	if got != "https://store.example.com/uploads/demo.png" {
		t.Fatalf("unexpected resolved asset URL: %s", got)
	}

	absolute := "https://cdn.example.com/logo.png"
	if got := resolveAppStoreAssetURL(absolute, "https://store.example.com"); got != absolute {
		t.Fatalf("absolute URL should be unchanged: %s", got)
	}
}

func TestBuildAppStoreParamsDotenvOverridesComposeDefault(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:${NGINX_TAG:-latest}
    ports:
      - "${WEB_PORT:-8080}:80"
`
	refs := extractComposeVarRefs(compose)
	params, _ := buildLegacyAppStoreParams(&App{Compose: compose}, refs, map[string]string{
		"WEB_PORT": "18080",
	}, map[string][]envFileRef{}, map[string]composeSecretDef{}, nil)

	values := map[string]string{}
	for _, param := range params {
		values[param.Key] = param.DefaultValue
	}

	if values["WEB_PORT"] != "18080" {
		t.Fatalf("expected dotenv value to override compose default, got %q", values["WEB_PORT"])
	}
	if values["NGINX_TAG"] != "latest" {
		t.Fatalf("expected compose default for NGINX_TAG, got %q", values["NGINX_TAG"])
	}
}
