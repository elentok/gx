// Reports which vendored skills have upstream changes and, with --merge, merges them into the gx
// copies. See docs/specs/vendored-skills.md. Node 24 runs this file directly.
//
// Usage: node scripts/sync-vendored-skills.ts --check|--merge [--accept-license SKILL]...
//        [--manifest FILE] [--skills-dir DIR]
//
// --check lists skills with upstream changes. --merge 3-way merges `merge` skills into the gx
// copies under --skills-dir (default: ../skills relative to this script's own directory, not the
// current directory) and reports; `port` skills only get the upstream diff. The pin does not move.
// A changed upstream LICENSE blocks its skill until a reviewer re-runs with --accept-license SKILL,
// which copies the new LICENSE. README.md is never touched.
//
// An upstream path that is a single file merges into <skills-dir>/<skill>/SKILL.md.
//
// GX_UPSTREAM_BASE replaces https://github.com as the place upstream repos are fetched from
// (<base>/<owner>/<repo>), so tests never touch the network.
import { spawnSync } from "node:child_process";
import {
	cpSync,
	existsSync,
	mkdirSync,
	mkdtempSync,
	readdirSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type Strategy = "merge" | "port";

interface Row {
	skill: string;
	repo: string;
	path: string;
	license: string;
	pin: string;
	strategy: Strategy;
}

const scriptDir = dirname(fileURLToPath(import.meta.url));
const requiredFields = ["skill", "repo", "path", "license", "pin"] as const;
const knownFields = new Set<string>([...requiredFields, "strategy"]);

class UsageError extends Error {}
class ManifestError extends Error {}

function parseManifest(text: string, file: string): Row[] {
	let data: unknown;
	try {
		data = JSON.parse(text);
	} catch (e) {
		throw new ManifestError(`${file}: not valid JSON: ${(e as Error).message}`);
	}
	if (!Array.isArray(data)) {
		throw new ManifestError(`${file}: expected a JSON array of rows`);
	}
	return data.map((raw: unknown, i: number): Row => {
		const label = (name?: unknown) =>
			`${file}: row ${i + 1}${typeof name === "string" && name ? ` (${name})` : ""}`;
		if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
			throw new ManifestError(`${label()}: expected an object`);
		}
		const obj = raw as Record<string, unknown>;
		for (const key of Object.keys(obj)) {
			if (!knownFields.has(key)) {
				throw new ManifestError(`${label(obj.skill)}: unknown field "${key}"`);
			}
		}
		for (const field of requiredFields) {
			const v = obj[field];
			if (typeof v !== "string" || v.trim() === "") {
				throw new ManifestError(`${label(obj.skill)}: field "${field}" must be a non-empty string`);
			}
		}
		const strategy = obj.strategy ?? "merge";
		if (strategy !== "merge" && strategy !== "port") {
			throw new ManifestError(`${label(obj.skill)}: field "strategy" must be "merge" or "port"`);
		}
		return { ...(obj as Omit<Row, "strategy">), strategy };
	});
}

function parseArgs(argv: string[]) {
	let mode = "";
	let manifest = join(scriptDir, "vendored-skills.json");
	let skillsDir = join(scriptDir, "..", "skills");
	const accepted = new Set<string>();
	const value = (i: number, what: string): string => {
		const v = argv[i + 1];
		if (v === undefined) throw new UsageError(`${argv[i]} needs ${what}`);
		return v;
	};
	for (let i = 0; i < argv.length; i++) {
		switch (argv[i]) {
			case "--check":
				mode = "check";
				break;
			case "--merge":
				mode = "merge";
				break;
			case "--accept-license":
				accepted.add(value(i++, "a skill name"));
				break;
			case "--manifest":
				manifest = value(i++, "a file");
				break;
			case "--skills-dir":
				skillsDir = value(i++, "a directory");
				break;
			default:
				throw new UsageError(`unknown argument: ${argv[i]}`);
		}
	}
	if (!mode) {
		throw new UsageError(
			"usage: sync-vendored-skills.ts --check|--merge [--accept-license SKILL]... [--manifest FILE] [--skills-dir DIR]",
		);
	}
	return { mode, manifest, skillsDir, accepted };
}

function git(dir: string, ...args: string[]): Buffer {
	const r = spawnSync("git", ["-C", dir, ...args], { maxBuffer: 1 << 30 });
	if (r.status !== 0) {
		throw new Error(`git ${args.join(" ")}: ${r.stderr?.toString() ?? r.error}`);
	}
	return r.stdout;
}

// The file's bytes at rev:path, or undefined when it does not exist there.
function show(dir: string, rev: string, path: string): Buffer | undefined {
	const r = spawnSync("git", ["-C", dir, "show", `${rev}:${path}`], { maxBuffer: 1 << 30 });
	return r.status === 0 ? r.stdout : undefined;
}

function filesUnder(dir: string, rev: string, path: string): string[] {
	const r = spawnSync("git", ["-C", dir, "ls-tree", "-r", "--name-only", `${rev}:${path}`], {
		maxBuffer: 1 << 30,
	});
	return r.status === 0 ? r.stdout.toString().split("\n").filter(Boolean) : [];
}

function localFiles(dir: string): string[] {
	if (!existsSync(dir)) return [];
	return readdirSync(dir, { recursive: true, withFileTypes: true })
		.filter((e) => e.isFile())
		.map((e) => relative(dir, join(e.parentPath, e.name)).split("\\").join("/"));
}

function sameBytes(a: Buffer, b: Buffer): boolean {
	return a.equals(b);
}

function readIfFile(path: string): Buffer | undefined {
	return existsSync(path) ? readFileSync(path) : undefined;
}

function unifiedDiff(a: string, b: string): string {
	return spawnSync("diff", ["-u", a, b], { encoding: "utf8" }).stdout;
}

function main(): void {
	const { mode, manifest, skillsDir, accepted } = parseArgs(process.argv.slice(2));
	const rows = parseManifest(readFileSync(manifest, "utf8"), manifest);
	const work = mkdtempSync(join(tmpdir(), "sync-vendored-"));
	try {
		run(rows, mode, skillsDir, accepted, work);
	} finally {
		rmSync(work, { recursive: true, force: true });
	}
}

const clones = new Map<string, string>();

// Full clone, so every pin is reachable. One clone per upstream repo per run.
function fetchRepo(work: string, repo: string): string {
	const cached = clones.get(repo);
	if (cached) return cached;
	const base = process.env.GX_UPSTREAM_BASE;
	const url = base ? `${base}/${repo}` : `https://github.com/${repo}.git`;
	const dir = join(work, repo.replaceAll("/", "__"));
	const r = spawnSync("git", ["clone", "--quiet", "--bare", url, dir], { encoding: "utf8" });
	if (r.status !== 0) throw new Error(`git clone ${url}: ${r.stderr}`);
	clones.set(repo, dir);
	return dir;
}

// 3-way merges one skill into gx's copy: base = upstream at the pin, ours = gx, theirs = latest.
// The pin does not move.
function mergeSkill(
	work: string,
	skill: string,
	dir: string,
	path: string,
	pin: string,
	latest: string,
	skillsDir: string,
): void {
	const oursDir = join(skillsDir, skill);
	const isFile = git(dir, "cat-file", "-t", `${latest}:${path}`).toString().trim() === "blob";
	// ours name -> upstream path
	const entries = new Map<string, string>();
	if (isFile) {
		entries.set("SKILL.md", path);
	} else {
		const names = new Set([
			...filesUnder(dir, pin, path),
			...filesUnder(dir, latest, path),
			...localFiles(oursDir),
		]);
		for (const f of [...names].sort()) {
			if (f === "LICENSE" || f === "README.md") continue;
			entries.set(f, `${path}/${f}`);
		}
	}

	const clean: string[] = [];
	const conflicted: string[] = [];
	const tmp = join(work, "merge");
	rmSync(tmp, { recursive: true, force: true });
	mkdirSync(tmp, { recursive: true });

	for (const [f, upPath] of entries) {
		const ours = join(oursDir, f);
		const base = show(dir, pin, upPath);
		const mine = readIfFile(ours);
		const theirs = show(dir, latest, upPath);
		// base, ours, theirs presence. 000, 010 and 100 need no action: gone everywhere, added only in
		// gx, or deleted on both sides.
		if (base && mine && theirs) {
			// In all three: 3-way merge unless upstream did not change it.
			if (sameBytes(base, theirs)) continue;
			const baseFile = join(tmp, "base");
			const theirsFile = join(tmp, "theirs");
			writeFileSync(baseFile, base);
			writeFileSync(theirsFile, theirs);
			const r = spawnSync("git", ["merge-file", ours, baseFile, theirsFile]);
			(r.status === 0 ? clean : conflicted).push(f);
		} else if (!base && !mine && theirs) {
			// Added upstream, so copy it in.
			mkdirSync(dirname(ours), { recursive: true });
			writeFileSync(ours, theirs);
			clean.push(f);
		} else if (!base && mine && theirs) {
			// Added by both gx and upstream, a conflict unless the contents match.
			if (!sameBytes(mine, theirs)) conflicted.push(f);
		} else if (base && mine && !theirs) {
			// Deleted upstream, so delete it here too unless gx changed it (then a conflict).
			if (sameBytes(mine, base)) {
				rmSync(ours);
				clean.push(f);
			} else {
				conflicted.push(f);
			}
		} else if (base && !mine && theirs) {
			// Deleted in gx, a conflict if upstream changed the file since the pin.
			if (!sameBytes(base, theirs)) conflicted.push(f);
		}
	}

	console.log("clean files:");
	for (const f of clean) console.log(`  ${f}`);
	console.log("conflicted files:");
	for (const f of conflicted) console.log(`  ${f}`);
}

// Returns 0 when the skill may be synced, 1 when it is blocked, 2 when synced but reported. A
// changed upstream LICENSE blocks the skill unless it was named with --accept-license; an accepted
// one is copied word for word. --check only reports it.
function licenseGate(
	work: string,
	mode: string,
	row: Row,
	dir: string,
	latest: string,
	skillsDir: string,
	accept: boolean,
): number {
	const oldFile = join(work, "license.old");
	const newFile = join(work, "license.new");
	const oldBytes = show(dir, row.pin, row.license) ?? Buffer.alloc(0);
	const newBytes = show(dir, latest, row.license) ?? Buffer.alloc(0);
	if (sameBytes(oldBytes, newBytes)) return 0;
	writeFileSync(oldFile, oldBytes);
	writeFileSync(newFile, newBytes);
	if (mode === "check") {
		console.log(`${row.skill}: license changed`);
		process.stdout.write(unifiedDiff(oldFile, newFile));
		return 2;
	}
	if (accept) {
		mkdirSync(join(skillsDir, row.skill), { recursive: true });
		cpSync(newFile, join(skillsDir, row.skill, "LICENSE"));
		console.log(`${row.skill}: license change accepted, LICENSE updated`);
		return 2;
	}
	console.log(`${row.skill}: license changed; nothing written for this skill`);
	process.stdout.write(unifiedDiff(oldFile, newFile));
	return 1;
}

function run(rows: Row[], mode: string, skillsDir: string, accepted: Set<string>, work: string): void {
	let found = false;
	for (const row of rows) {
		const { skill, repo, path, pin, strategy } = row;
		const dir = fetchRepo(work, repo);
		const latest = git(dir, "rev-parse", "HEAD").toString().trim();
		if (spawnSync("git", ["-C", dir, "cat-file", "-e", `${latest}:${path}`]).status !== 0) {
			found = true;
			console.log(`${skill}: upstream path ${path} no longer exists at ${latest}; nothing written for this skill`);
			continue;
		}
		const gate = licenseGate(work, mode, row, dir, latest, skillsDir, accepted.has(skill));
		if (gate !== 0) found = true;
		if (gate === 1) continue;
		const count = Number(git(dir, "rev-list", "--count", `${pin}..${latest}`, "--", path).toString().trim());
		if (count === 0) continue;
		found = true;
		if (mode === "check") {
			console.log(`${skill}: ${count} new upstream commits since ${pin}`);
			continue;
		}
		console.log(`== ${skill}: ${count} new upstream commits since ${pin} ==`);
		process.stdout.write(git(dir, "log", "--oneline", `${pin}..${latest}`, "--", path));
		console.log("upstream diff:");
		process.stdout.write(git(dir, "diff", pin, latest, "--", path));
		if (strategy === "merge") mergeSkill(work, skill, dir, path, pin, latest, resolve(skillsDir));
	}
	if (!found) console.log("All vendored skills are up to date.");
}

try {
	main();
} catch (e) {
	if (e instanceof UsageError) {
		console.error(e.message);
		process.exit(2);
	}
	console.error(e instanceof ManifestError ? e.message : `sync-vendored-skills: ${(e as Error).message}`);
	process.exit(1);
}
