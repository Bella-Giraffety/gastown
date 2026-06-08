// Gas Town OpenCode plugin: hooks SessionStart/Compaction via events.
// Injects gt prime context into the system prompt via experimental.chat.system.transform.
export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  const MAX_COMPACTIONS = 3;
  let didInit = false;
  let compactionCount = 0;
  let cycleInProgress = false;
  let handoffSaved = false;

  // Promise-based context loading ensures the system transform hook can
  // await the result even if session.created hasn't resolved yet.
  let primePromise = null;

  const roleKind = () => {
    const parts = role.split("/").filter(Boolean);
    if (parts.length >= 2 && parts[1] === "polecats") return "polecat";
    if (parts.length >= 2 && parts[1] === "crew") return "crew";
    if (parts.length >= 2) return parts[1];
    return parts[0] || "";
  };

  const safeSessionName = (value) => /^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(value);

  const captureRun = async (cmd) => {
    try {
      // .text() captures stdout as a string and suppresses terminal echo.
      return await $`/bin/sh -lc ${cmd}`.cwd(directory).text();
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
      return "";
    }
  };

  const loadPrime = async () => {
    const context = await captureRun("gt prime --hook");
    // NOTE: session-started nudge to deacon removed — it interrupted
    // the deacon's await-signal backoff. Deacon wakes on beads activity.
    return context;
  };

  const signalSessionCycle = async () => {
    if (cycleInProgress) return;
    cycleInProgress = true;
    try {
      await $`gt costs record`.cwd(directory).catch(() => {});
      if (!handoffSaved) {
        const subject = "OpenCode compaction cycle";
        const message = `Compacted ${compactionCount} times; context snapshot for successor`;
        await $`gt handoff --auto -s ${subject} -m ${message}`.cwd(directory);
        handoffSaved = true;
      }

      const sessionName = process.env.GT_SESSION || "";
      if (!sessionName) {
        console.error("[gastown] opencode compaction cycle saved handoff but GT_SESSION is unset; cannot kill tmux session");
        cycleInProgress = false;
        return;
      }
      if (!safeSessionName(sessionName)) {
        console.error(`[gastown] refusing to kill tmux session with unsafe GT_SESSION=${sessionName}`);
        cycleInProgress = false;
        return;
      }

      await $`tmux kill-session -t ${"=" + sessionName}`.cwd(directory);
      console.error(`[gastown] opencode compaction cycle killed ${sessionName} after ${compactionCount} compactions`);
    } catch (err) {
      console.error("[gastown] opencode compaction cycle failed", err?.message || err);
      cycleInProgress = false;
    }
  };

  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        if (didInit) return;
        didInit = true;
        compactionCount = 0;
        cycleInProgress = false;
        handoffSaved = false;
        // Start loading prime context early; system.transform will await it.
        primePromise = loadPrime();
      }
      if (event?.type === "session.compacted") {
        // Reset so next system.transform gets fresh context.
        compactionCount++;
        primePromise = loadPrime();
        if (autonomousRoles.has(roleKind()) && compactionCount >= MAX_COMPACTIONS) {
          await signalSessionCycle();
        }
      }
      if (event?.type === "session.deleted") {
        const sessionID = event.properties?.info?.id;
        if (sessionID) {
          await $`gt costs record --session ${sessionID}`.catch(() => {});
        }
      }
    },
    "experimental.chat.system.transform": async (input, output) => {
      // If session.created hasn't fired yet, start loading now.
      if (!primePromise) {
        primePromise = loadPrime();
      }
      const context = await primePromise;
      if (context) {
        output.system.push(context);
      } else {
        // Reset so next transform retries instead of pushing empty forever.
        primePromise = null;
      }
    },
    "experimental.session.compacting": async ({ sessionID }, output) => {
      const roleDisplay = roleKind() || "unknown";
      const willCycle = autonomousRoles.has(roleKind()) && compactionCount + 1 >= MAX_COMPACTIONS;
      output.context.push(`
## Gas Town Multi-Agent System

**After Compaction:** Run \`gt prime --hook\` to restore full context.
**Check Hook:** \`gt hook\` - if work present, execute immediately (GUPP).
**Role:** ${roleDisplay}${willCycle ? `\n**Session Cycle:** OpenCode will save handoff state and restart after this compaction.` : ""}
`);
    },
  };
};
