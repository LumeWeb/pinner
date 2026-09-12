// Behavioral tests for the download entry's DOM wiring (src/download-bootstrap.ts):
// the form-submit sink derivation honoring config.dropAvailable, and the result
// readout (outLink/outPath) lifecycle — written when the machine reaches Ok and
// cleared on reset/error so a repeat or failed download never leaves a stale
// "Save file" link pointing at a previous fetch_url.

import { beforeEach, describe, expect, it } from "vitest";
import type { CallTool, ToolResult } from "@lumeweb/mcpcanvas";
import { runDownloadEntry, type DownloadElements } from "@/download-bootstrap";
import type { DownloadConfig } from "@/download";
import { untilState } from "./helpers";

const baseConfig: DownloadConfig = {
  downloadTool: "download_file",
  sourceArg: "ipfs_path",
  downloadingMsg: "Downloading...",
  downloadedMsg: "Downloaded.",
  failedMsg: "Download failed.",
  noSourceMsg: "Enter a CID.",
  dropAvailable: true,
};

type Handler = (ev: { preventDefault(): void }) => void;

function makeElements() {
  const outLink = { href: undefined as string | undefined, textContent: "", style: { display: "none" } };
  const outPath = { textContent: "", style: { display: "none" } };
  let formHandler: Handler = () => {};
  const elements: DownloadElements = {
    form: { addEventListener(_t: "submit", fn: Handler) { formHandler = fn; } },
    sourceInput: { value: "bafy/src.txt" },
    nameInput: { value: "" },
    outputInput: { value: "" },
    sinkLocal: { checked: true },
    sinkDrop: { checked: false },
    statusEl: {} as HTMLElement,
    outLink: outLink as DownloadElements["outLink"],
    outPath: outPath as unknown as DownloadElements["outPath"],
    startBtn: {} as DownloadElements["startBtn"],
  };
  const fire = () => formHandler({ preventDefault() {} });
  return { elements, fire };
}

function scriptedCallTool(calls: { name: string; arguments: Record<string, unknown> }[]): CallTool {
  return (req: Parameters<CallTool>[0]) => {
    calls.push({ name: req.name, arguments: req.arguments as Record<string, unknown> });
    return new Promise<ToolResult>(() => {});
  };
}

describe("download bootstrap form submit sink derivation", () => {
  let calls: { name: string; arguments: Record<string, unknown> }[];

  beforeEach(() => {
    calls = [];
  });

  it("dropAvailable:false forces sink=local even when the drop radio is checked", () => {
    const { elements, fire } = makeElements();
    elements.sinkDrop.checked = true; // checked but unavailable
    runDownloadEntry({ config: { ...baseConfig, dropAvailable: false }, callTool: scriptedCallTool(calls), elements });
    fire();
    expect(calls.length).toBe(1);
    expect(calls[0].arguments.sink).toBe("local");
  });

  it("dropAvailable:true honors a checked drop radio", () => {
    const { elements, fire } = makeElements();
    elements.sinkDrop.checked = true;
    runDownloadEntry({ config: baseConfig, callTool: scriptedCallTool(calls), elements });
    fire();
    expect(calls.length).toBe(1);
    expect(calls[0].arguments.sink).toBe("drop");
  });
});

describe("download bootstrap readout lifecycle", () => {
  function okCallTool(): CallTool {
    return () =>
      new Promise<ToolResult>((resolve) => {
        resolve({ structuredContent: { status: "ok", sink: "drop", fetch_url: "http://host/dl/tok" } });
      });
  }

  it("writes outLink on Ok, then clears it on reset", async () => {
    const { elements, fire } = makeElements();
    elements.sinkDrop.checked = true;
    const entry = runDownloadEntry({ config: baseConfig, callTool: okCallTool(), elements });
    fire();
    await untilState(entry.service, "ok");

    const link = elements.outLink as { href?: string; textContent: string; style: { display: string } };
    expect(link.href).toBe("http://host/dl/tok");
    expect(link.textContent).toBe("Save file");
    expect(link.style.display).toBe("");

    entry.service.send({ type: "reset" });
    expect(link.href).toBeUndefined();
    expect(link.textContent).toBe("");
    expect(link.style.display).toBe("none");
  });
});
