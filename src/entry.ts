// AssistClaw TypeScript entry point
// This file is the Node.js CLI shim — it loads environment variables,
// validates prerequisites, and spawns the agent core.
// The primary binary is the Go orchestrator; this provides TypeScript
// integration for agents using the pi-agent-core SDK.

import { normalizeEnv, installProcessWarningFilter } from "@mariozechner/pi-agent-core";
import { resolve } from "path";

process.title = "assistclaw";

installProcessWarningFilter();
normalizeEnv();

// Ensure ASSISTCLAW env vars take priority, but also accept legacy OPENCLAW vars
// and the standard provider-native vars (OPENAI_API_KEY, ANTHROPIC_API_KEY etc.)
const envAliases: Record<string, string> = {
    ASSISTCLAW_OPENAI_API_KEY: "OPENAI_API_KEY",
    ASSISTCLAW_ANTHROPIC_API_KEY: "ANTHROPIC_API_KEY",
    ASSISTCLAW_GROQ_API_KEY: "GROQ_API_KEY",
    ASSISTCLAW_MISTRAL_API_KEY: "MISTRAL_API_KEY",
    ASSISTCLAW_OPENROUTER_API_KEY: "OPENROUTER_API_KEY",
};

for (const [assistclawVar, standardVar] of Object.entries(envAliases)) {
    if (process.env[assistclawVar] && !process.env[standardVar]) {
        process.env[standardVar] = process.env[assistclawVar];
    }
}

// Re-spawn with required Node.js flags if missing
const requiredFlags = ["--experimental-vm-modules", "--no-warnings"];
const hasAllFlags = requiredFlags.every((f) => process.execArgv.includes(f));

if (!hasAllFlags) {
    const { spawn } = await import("child_process");
    const child = spawn(
        process.execPath,
        [...(process.execArgv ?? []), ...requiredFlags, ...process.argv.slice(1)],
        { stdio: "inherit", env: process.env }
    );
    child.on("exit", (code) => process.exit(code ?? 0));
} else {
    import("./cli/run-main.js")
        .then(({ runCli }) => runCli(process.argv))
        .catch((error: unknown) => {
            console.error("Fatal error starting AssistClaw:", error);
            process.exit(1);
        });
}
