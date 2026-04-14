// Gas Town OpenCode plugin: hooks SessionStart/Compaction via events.
// Injects gt prime context into the system prompt via experimental.chat.system.transform.
export const GasTown = async ({ client, $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  const refusalText = "I'm sorry, but I cannot assist with that request.";
  const refusalPrompt = "Continue";
  const maxConsecutiveRefusalRetries = 4;
  let didInit = false;
  const refusalState = new Map();

  // Promise-based context loading ensures the system transform hook can
  // await the result even if session.created hasn't resolved yet.
  let primePromise = null;

  const log = async (level, message, extra = {}) => {
    try {
      await client?.app?.log?.({
        body: {
          service: "gastown-opencode-plugin",
          level,
          message,
          extra,
        },
      });
    } catch {}
  };

  const getSessionState = (sessionID) => {
    if (!refusalState.has(sessionID)) {
      refusalState.set(sessionID, {
        consecutiveRefusals: 0,
        didEscalate: false,
        lastHandledMessageID: "",
      });
    }
    return refusalState.get(sessionID);
  };

  const captureRun = async (cmd) => {
    try {
      // .text() captures stdout as a string and suppresses terminal echo.
      return await $`/bin/sh -lc ${cmd}`.cwd(directory).text();
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
      return "";
    }
  };

  const fetchAssistantText = async (sessionID, messageID) => {
    try {
      const response = await client?.session?.message?.({
        path: { id: sessionID, messageID },
      });
      const data = response?.data || response;
      const parts = Array.isArray(data?.parts) ? data.parts : [];
      return parts
        .filter((part) => part?.type === "text" && !part?.ignored)
        .map((part) => part.text || "")
        .join("")
        .trim();
    } catch (err) {
      await log("warn", "failed to fetch completed assistant text", {
        sessionID,
        messageID,
        error: err?.message || String(err),
      });
      return null;
    }
  };

  const promptSession = async (sessionID, text) => {
    const body = {
      parts: [{ type: "text", text }],
    };
    if (typeof client?.session?.promptAsync === "function") {
      await client.session.promptAsync({
        path: { id: sessionID },
        body,
      });
      return;
    }
    if (typeof client?.session?.prompt === "function") {
      await client.session.prompt({
        path: { id: sessionID },
        body,
      });
    }
  };

  const escalateRefusalLoop = async (sessionID, count) => {
    const summary = "OpenCode refusal auto-continue exhausted";
    const details = [
      `role=${role || "unknown"}`,
      `session=${sessionID}`,
      `refusals=${count}`,
      `prompt=${JSON.stringify(refusalPrompt)}`,
    ].join(" ");

    await log("warn", summary, { sessionID, consecutiveRefusals: count, role });

    try {
      await $`gt escalate ${summary} -s HIGH -m ${details}`.cwd(directory);
    } catch (err) {
      console.error("[gastown] gt escalate failed", err?.message || err);
    }
  };

  const handleCompletedAssistantMessage = async (info) => {
    const sessionID = info?.sessionID;
    const messageID = info?.id;
    if (!sessionID || !messageID || info?.role !== "assistant" || !info?.time?.completed) {
      return;
    }

    const state = getSessionState(sessionID);
    if (state.lastHandledMessageID === messageID) {
      return;
    }
    state.lastHandledMessageID = messageID;

    const text = await fetchAssistantText(sessionID, messageID);
    if (text == null) {
      return;
    }

    if (text !== refusalText) {
      state.consecutiveRefusals = 0;
      state.didEscalate = false;
      return;
    }

    state.consecutiveRefusals += 1;
    if (state.consecutiveRefusals > maxConsecutiveRefusalRetries) {
      if (state.didEscalate) {
        return;
      }
      state.didEscalate = true;
      await escalateRefusalLoop(sessionID, state.consecutiveRefusals);
      return;
    }

    await log("info", "sending OpenCode refusal auto-continue prompt", {
      sessionID,
      consecutiveRefusals: state.consecutiveRefusals,
    });
    await promptSession(sessionID, refusalPrompt);
  };

  const loadPrime = async () => {
    let context = await captureRun("gt prime");
    if (autonomousRoles.has(role)) {
      const mail = await captureRun("gt mail check --inject");
      if (mail) {
        context += "\n" + mail;
      }
    }
    // NOTE: session-started nudge to deacon removed — it interrupted
    // the deacon's await-signal backoff. Deacon wakes on beads activity.
    return context;
  };

  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        if (didInit) return;
        didInit = true;
        // Start loading prime context early; system.transform will await it.
        primePromise = loadPrime();
      }
      if (event?.type === "session.compacted") {
        // Reset so next system.transform gets fresh context.
        primePromise = loadPrime();
      }
      if (event?.type === "message.updated") {
        await handleCompletedAssistantMessage(event.properties?.info);
      }
      if (event?.type === "session.deleted") {
        const sessionID = event.properties?.info?.id;
        if (sessionID) {
          refusalState.delete(sessionID);
        }
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
      const roleDisplay = role || "unknown";
      output.context.push(`
## Gas Town Multi-Agent System

**After Compaction:** Run \`gt prime\` to restore full context.
**Check Hook:** \`gt hook\` - if work present, execute immediately (GUPP).
**Role:** ${roleDisplay}
`);
    },
  };
};
