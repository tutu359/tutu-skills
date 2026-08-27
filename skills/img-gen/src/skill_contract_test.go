package main

import (
	"os"
	"strings"
	"testing"
)

func readSkillContract(t *testing.T) (string, string) {
	t.Helper()
	skill, err := os.ReadFile("../SKILL.md")
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	batch, err := os.ReadFile("../references/batch-format.md")
	if err != nil {
		t.Fatalf("read batch-format.md: %v", err)
	}
	return string(skill), string(batch)
}

func requireDocFragments(t *testing.T, doc string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(doc, fragment) {
			t.Errorf("documentation is missing required contract fragment %q", fragment)
		}
	}
}

func TestProviderReferenceDocumentsOpenAIFactsOnly(t *testing.T) {
	data, err := os.ReadFile("../references/providers/openai.md")
	if err != nil {
		t.Fatalf("read OpenAI Provider reference: %v", err)
	}
	doc := string(data)
	requireDocFragments(t, doc, "# OpenAI Provider", "baseURL", "apiKey", "model", "/v1/images/generations", "/v1/images/edits", "b64_json")
	if strings.Contains(doc, "IMAGE_API_") || strings.Contains(doc, "--concurrency") || strings.Contains(doc, "chmod +x") {
		t.Fatal("OpenAI Provider reference duplicates common troubleshooting rules")
	}
}

func TestProviderReferenceDocumentsGoogleFactsOnly(t *testing.T) {
	data, err := os.ReadFile("../references/providers/google.md")
	if err != nil {
		t.Fatalf("read Google Provider reference: %v", err)
	}
	doc := string(data)
	requireDocFragments(t, doc, "# Google Provider", "baseURL", "apiKey", "model", "/v1beta/models/<model>:predict", "instances", "predictions", "bytesBase64Encoded")
	if strings.Contains(doc, "IMAGE_API_") || strings.Contains(doc, "--concurrency") || strings.Contains(doc, "chmod +x") || strings.Contains(doc, "common troubleshooting") {
		t.Fatal("Google Provider reference duplicates common troubleshooting rules")
	}
}

func TestCommonTroubleshootingIsTheFirstFailureReference(t *testing.T) {
	data, err := os.ReadFile("../references/troubleshooting.md")
	if err != nil {
		t.Fatalf("read troubleshooting reference: %v", err)
	}
	doc := string(data)
	requireDocFragments(t, doc,
		"first reference after every",
		"missing or unusable Provider Configuration",
		"run `<img-gen> init`",
		"permission-denied error",
		"network failures, timeouts",
		"all HTTP `5xx`",
		"Do not retry any `4xx`",
		"one global worker pool",
		"--fail-fast",
	)
	if strings.Contains(doc, "test-key") || strings.Contains(doc, "json-key") || strings.Contains(doc, "google-key") {
		t.Fatal("common troubleshooting contains a test or credential value")
	}
}

func TestProviderReferencesExcludeCommonFailureRules(t *testing.T) {
	for _, provider := range []string{"openai", "google"} {
		data, err := os.ReadFile("../references/providers/" + provider + ".md")
		if err != nil {
			t.Fatalf("read %s Provider reference: %v", provider, err)
		}
		doc := strings.ToLower(string(data))
		for _, fragment := range []string{"permission", "chmod", "network", "timeout", "retry", "concurrency", "batch"} {
			if strings.Contains(doc, fragment) {
				t.Errorf("%s Provider reference duplicates common failure rule %q", provider, fragment)
			}
		}
	}
}

func TestSkillDocumentsDeclareProgressiveReferenceDisclosure(t *testing.T) {
	skill, _ := readSkillContract(t)
	requireDocFragments(t, skill,
		"Normal `generate`, `edit`, and `batch` tasks do not read any reference document",
		"Do not pre-check configuration",
		"Read [references/troubleshooting.md](references/troubleshooting.md) first after every failure",
		"If common troubleshooting resolves the failure, stop there and do not read a Provider reference",
		"Read only the current Provider's reference",
		"OpenAI failures read only `references/providers/openai.md`",
		"Google failures read only `references/providers/google.md`",
	)
	retiredBatchCommand := "generate" + "-batch"
	if strings.Contains(skill, retiredBatchCommand) {
		t.Fatal("SKILL.md retains the retired batch command name")
	}
}

func TestSkillDocumentsDeclareGoogleProviderContract(t *testing.T) {
	skill, _ := readSkillContract(t)
	requireDocFragments(t, skill, "OpenAI or Google Provider", `"google":`, "imagen-3.0-generate-002")
}

func TestSkillDocumentsDeclareGenerationSetContract(t *testing.T) {
	skill, batch := readSkillContract(t)
	requireDocFragments(t, skill,
		"one **Generation Set**",
		"invoke `<img-gen> batch` exactly once",
		"Do not start one shell command per image",
		"repeat the prompt **exactly** in every job, including `edit` jobs",
		"Make only the `out` values unique",
		"Create a derived prompt only for a dimension the user explicitly authorizes",
		"the user must authorize a dimension such as style, pose, or scene",
		"Once a dimension is authorized, choose concrete values when the user leaves them open",
		"When an authorized dimension has no concrete value, choose suitable values and generate directly",
		"Every derived prompt must be self-contained",
		"Performance or rendering variants",
		"World expansion",
		"person, character, object, scene, background, environment, palette, or style",
		"Consistency Anchor",
		"A reference image is a stronger visual anchor than prose",
		"use the explicit `edit` operation",
		"Do not inspect input images on the normal path",
		"do not inspect, score, select, compare for similarity, or visually retry",
		"Treat the user's prompt as authoritative",
		"control plane",
		"execution plane",
		"Batch concurrency is `--concurrency`, or the CLI default of `5`.",
		"The CLI preflights every operation",
		"`--fail-fast` stops scheduling jobs not yet started",
		"deliver each successful output immediately",
	)
	requireDocFragments(t, batch,
		"Every job must explicitly declare `operation`",
		"Do not add `n` or another quantity field",
		"Images are uploaded in exactly the array order",
		"Relative `image` and `mask` paths are resolved",
		"Relative `out` paths are resolved under the command's `--out-dir`",
		"The CLI validates every operation",
		"Concurrency is selected by `--concurrency`, then the default `5`.",
		"successful files are kept",
		"process exits nonzero if any job fails",
	)
}

func TestSkillDocumentsContainRequiredBatchFixtures(t *testing.T) {
	skill, _ := readSkillContract(t)
	requireDocFragments(t, skill,
		`{"operation":"generate","prompt":"A small blue nebula in a glass bottle, studio product photo","out":"nebula-01.png"}`,
		`{"operation":"generate","prompt":"A small blue nebula in a glass bottle, studio product photo","out":"nebula-02.png"}`,
		`{"operation":"generate","prompt":"A red paper kite above a coastal cliff at sunrise; landscape composition, clear sky, no text; render in watercolor"`,
		`{"operation":"generate","prompt":"A red paper kite above a coastal cliff at sunrise; landscape composition, clear sky, no text; render in charcoal"`,
		`{"operation":"generate","prompt":"The same floating city of brass bridges, blue glass towers, and perpetual twilight; show the crowded eastern market`,
		`{"operation":"generate","prompt":"The same floating city of brass bridges, blue glass towers, and perpetual twilight; show the quiet western observatory`,
		`{"operation":"edit","prompt":"Combine the supplied images into one editorial collage. Preserve the user's composition and do not add text.","image":["inputs/subject.png","inputs/texture.png","inputs/layout.png"],"out":"collage-01.png"}`,
	)
}
