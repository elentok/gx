import { describe, expect, it } from "vitest";
import { MAX_IMAGE_BYTES, MSG_BAD_TYPE, MSG_MAX_IMAGES, MSG_TOO_LARGE, checkImages } from "./imageLimits";

const PNG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
const JPEG = [0xff, 0xd8, 0xff, 0xe0];
const GIF = [0x47, 0x49, 0x46, 0x38, 0x39, 0x61];
const WEBP = [0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50];
const HEIC = [0, 0, 0, 0x18, 0x66, 0x74, 0x79, 0x70, 0x68, 0x65, 0x69, 0x63];

const file = (bytes: number[] | string, name = "x.bin") => new File([typeof bytes === "string" ? bytes : new Uint8Array(bytes)], name);

describe("checkImages", () => {
  it.each([
    ["PNG", PNG],
    ["JPEG", JPEG],
    ["GIF", GIF],
    ["WebP", WEBP],
  ])("accepts %s bytes", async (_, bytes) => {
    const r = await checkImages([file(bytes)], 0);
    expect(r.accepted).toHaveLength(1);
    expect(r.error).toBe("");
  });

  it("decides by bytes: a PNG named a.txt passes, a renamed text file does not", async () => {
    expect((await checkImages([file(PNG, "a.txt")], 0)).accepted).toHaveLength(1);
    const r = await checkImages([file("hello world, not an image", "a.png")], 0);
    expect(r.accepted).toHaveLength(0);
    expect(r.error).toBe(MSG_BAD_TYPE);
  });

  it.each([
    ["an SVG", file('<svg xmlns="http://www.w3.org/2000/svg"/>', "a.svg")],
    ["HEIC bytes", file(HEIC, "a.heic")],
  ])("refuses %s", async (_, f) => {
    const r = await checkImages([f], 0);
    expect(r.accepted).toHaveLength(0);
    expect(r.error).toBe(MSG_BAD_TYPE);
  });

  it("refuses a file over 20 MB", async () => {
    const big = new File([new Uint8Array(MAX_IMAGE_BYTES + 1)], "big.png");
    const r = await checkImages([big], 0);
    expect(r.accepted).toHaveLength(0);
    expect(r.error).toBe(MSG_TOO_LARGE);
  });

  it("adds the first 10 and refuses the 11th", async () => {
    const r = await checkImages(Array.from({ length: 11 }, () => file(PNG)), 0);
    expect(r.accepted).toHaveLength(10);
    expect(r.error).toBe(MSG_MAX_IMAGES);
  });

  it("counts images already in the draft", async () => {
    const r = await checkImages([file(PNG), file(PNG)], 9);
    expect(r.accepted).toHaveLength(1);
    expect(r.error).toBe(MSG_MAX_IMAGES);
  });
});
