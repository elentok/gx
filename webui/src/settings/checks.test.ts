import { describe, expect, it } from "vitest";
import { checkError, checksErrors, checksToModel, modelToChange, modelToChecks } from "./checks";

const roundTrip = (value: Record<string, unknown>) => modelToChecks(checksToModel(value));

describe("checks model round trips", () => {
  it.each([
    ["off", { lint: false }],
    ["one command", { format: "gofmt -l {files}" }],
    ["changed + full", { typecheck: { changed: "tsc {files}", full: "tsc" } }],
    ["full only", { lint: { full: "golangci-lint run" } }],
    ["changed + full + always", { lint: { changed: "a", full: "b", always: ["x", "y"] } }],
    ["test limits", { test: { full: "go test", limits: { suite: "4m", hang: "1m", slow: "20s" } } }],
    ["test per-tool slow", { test: { changed: "a", full: "b", limits: { slow: { go: "5s", vitest: "2s" } } } }],
    ["every kind", { format: "f", lint: false, typecheck: { full: "t" }, test: "go test" }],
  ])("%s", (_name, value) => {
    expect(roundTrip(value)).toEqual(value);
  });

  it("an absent value or key is the default form", () => {
    const model = checksToModel(undefined);
    expect(model.lint.mode).toBe("default");
    expect(modelToChecks(model)).toBeUndefined();
  });

  it("an all-default editor produces a remove change", () => {
    expect(modelToChange(checksToModel({}))).toEqual({ key: "checks", remove: true });
  });

  it("a set editor produces a set change", () => {
    expect(modelToChange(checksToModel({ lint: false }))).toEqual({ key: "checks", value: { lint: false } });
  });
});

describe("slow limit", () => {
  it("keeps the per-tool map form and drops emptied tools", () => {
    const model = checksToModel({ test: { full: "x", limits: { slow: { go: "5s", vitest: "2s" } } } });
    expect(model.test.slowIsMap).toBe(true);
    model.test.slowMap.go = "";
    expect(modelToChecks(model)).toEqual({ test: { full: "x", limits: { slow: { vitest: "2s" } } } });
  });

  it("keeps a single duration as a string", () => {
    const model = checksToModel({ test: { full: "x", limits: { slow: "9s" } } });
    expect(model.test.slowIsMap).toBe(false);
    expect(modelToChecks(model)).toEqual({ test: { full: "x", limits: { slow: "9s" } } });
  });

  it("clearing a field removes that limit, and the limits object when none are left", () => {
    const model = checksToModel({ test: { full: "x", limits: { suite: "4m", slow: "9s" } } });
    model.test.slow = "";
    expect(modelToChecks(model)).toEqual({ test: { full: "x", limits: { suite: "4m" } } });
    model.test.suite = "";
    expect(modelToChecks(model)).toEqual({ test: { full: "x" } });
  });

  it("clearing every map entry removes slow", () => {
    const model = checksToModel({ test: { full: "x", limits: { slow: { go: "5s" } } } });
    model.test.slowMap.go = "";
    expect(modelToChecks(model)).toEqual({ test: { full: "x" } });
  });

  it("a one-command test with limits saves as the object form", () => {
    const model = checksToModel({ test: "go test" });
    model.test.suite = "4m";
    expect(modelToChecks(model)).toEqual({ test: { changed: "go test", full: "go test", limits: { suite: "4m" } } });
  });

  it("limits apply to the test kind only", () => {
    const model = checksToModel({ lint: { full: "x" } });
    model.lint.suite = "4m";
    expect(modelToChecks(model)).toEqual({ lint: { full: "x" } });
  });
});

describe("always", () => {
  it("is written only on the changed + full form", () => {
    const model = checksToModel({ lint: { changed: "a", full: "b", always: ["x"] } });
    model.lint.mode = "one";
    model.lint.command = "c";
    expect(modelToChecks(model)).toEqual({ lint: "c" });
    model.lint.mode = "off";
    expect(modelToChecks(model)).toEqual({ lint: false });
  });
});

describe("validation", () => {
  it("flags changed without full", () => {
    const model = checksToModel({});
    model.lint.mode = "pair";
    model.lint.changed = "a";
    expect(checkError("lint", model.lint)).toMatch(/full/);
    expect(checksErrors(model)).toHaveProperty("lint");
  });

  it("flags an empty command and an empty always entry", () => {
    const model = checksToModel({ lint: "x", format: { full: "f", always: [""] } });
    model.lint.command = " ";
    expect(checkError("lint", model.lint)).toBeDefined();
    expect(checkError("format", model.format)).toBeDefined();
  });

  it("flags limits on a default test kind", () => {
    const model = checksToModel({});
    model.test.suite = "4m";
    expect(checkError("test", model.test)).toBeDefined();
  });

  it("accepts every valid form", () => {
    const model = checksToModel({ format: "f", lint: false, typecheck: { full: "t" }, test: { changed: "a", full: "b" } });
    expect(checksErrors(model)).toEqual({});
  });
});
