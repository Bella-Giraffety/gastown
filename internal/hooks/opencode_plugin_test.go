package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenCodePluginRefusalAutoContinue(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}

	pluginSource, err := templateFS.ReadFile("templates/opencode/gastown.js")
	if err != nil {
		t.Fatalf("read opencode template: %v", err)
	}

	dir := t.TempDir()
	pluginPath := filepath.Join(dir, "gastown-plugin.mjs")
	if err := os.WriteFile(pluginPath, pluginSource, 0644); err != nil {
		t.Fatalf("write plugin template: %v", err)
	}

	scriptPath := filepath.Join(dir, "harness.mjs")
	script := `
import assert from "node:assert/strict";
import { pathToFileURL } from "node:url";

const { GasTown } = await import(pathToFileURL(process.argv[2]).href);
const refusal = "I'm sorry, but I cannot assist with that request.";
const nonRefusal = "Implemented.";

const buildShell = (commands) => (strings, ...values) => {
  const cmd = strings.reduce(
    (acc, part, index) => acc + part + (index < values.length ? String(values[index]) : ""),
    "",
  );
  return {
    dir: "",
    cwd(dir) {
      this.dir = dir;
      return this;
    },
    async text() {
      commands.push({ cmd, dir: this.dir, mode: "text" });
      return "";
    },
    catch() {
      commands.push({ cmd, dir: this.dir, mode: "catch" });
      return Promise.resolve("");
    },
    then(resolve, reject) {
      commands.push({ cmd, dir: this.dir, mode: "exec" });
      return Promise.resolve("").then(resolve, reject);
    },
  };
};

const makePlugin = async (role) => {
  process.env.GT_ROLE = role;
  const prompts = [];
  const commands = [];
  const logs = [];
  const messages = new Map();

  const plugin = await GasTown({
    client: {
      session: {
        async message({ path }) {
          return {
            data: {
              info: {
                id: path.messageID,
                sessionID: path.id,
                role: "assistant",
                time: { completed: Date.now() },
              },
              parts: messages.get(path.messageID) ?? [],
            },
          };
        },
        async promptAsync(request) {
          prompts.push(request);
        },
      },
      app: {
        async log(entry) {
          logs.push(entry);
        },
      },
    },
    $: buildShell(commands),
    directory: process.cwd(),
  });

  return { plugin, prompts, commands, logs, messages };
};

const emitCompleted = async (ctx, sessionID, messageID, text) => {
  ctx.messages.set(messageID, [{ type: "text", text }]);
  await ctx.plugin.event({
    event: {
      type: "message.updated",
      properties: {
        info: {
          id: messageID,
          sessionID,
          role: "assistant",
          time: { created: 1, completed: 2 },
        },
      },
    },
  });
};

const polecat = await makePlugin("polecat");
await emitCompleted(polecat, "s1", "m1", refusal);
assert.equal(polecat.prompts.length, 1);
assert.equal(polecat.prompts[0].path.id, "s1");
assert.equal(polecat.prompts[0].body.parts[0].text, "Continue");

await emitCompleted(polecat, "s1", "m1", refusal);
assert.equal(polecat.prompts.length, 1);

await emitCompleted(polecat, "s1", "m2", nonRefusal);
await emitCompleted(polecat, "s1", "m3", refusal);
assert.equal(polecat.prompts.length, 2);

const capped = await makePlugin("polecat");
for (let i = 1; i <= 6; i += 1) {
  await emitCompleted(capped, "s2", "r" + i, refusal);
}
assert.equal(capped.prompts.length, 4);
assert.equal(capped.commands.filter((entry) => entry.cmd.includes("gt escalate")).length, 1);

const crew = await makePlugin("crew");
await emitCompleted(crew, "s3", "m1", refusal);
assert.equal(crew.prompts.length, 0);
`
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatalf("write node harness: %v", err)
	}

	cmd := exec.Command(node, scriptPath, pluginPath)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, string(output))
	}
}
