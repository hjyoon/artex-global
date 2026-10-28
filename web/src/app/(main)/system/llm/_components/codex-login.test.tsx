import "@testing-library/jest-dom/vitest";

import * as React from "react";

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api } from "@/lib/api";
import type { CodexLoginStatus } from "@/lib/types";

import messages from "../../../../../../messages/en.json";
import { CodexLogin } from "./codex-login";

vi.mock("@/lib/api", () => ({
  api: { codexStatus: vi.fn(), codexLogin: vi.fn(), codexCancel: vi.fn(), codexLogout: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const disconnected: CodexLoginStatus = { available: true, connected: false };
const pending: CodexLoginStatus = {
  ...disconnected,
  login: {
    login_id: "login-1",
    state: "pending",
    user_code: "TEST-CODE",
    verification_url: "https://auth.openai.com/codex/device",
  },
};
const connected: CodexLoginStatus = {
  available: true,
  connected: true,
  account: { email: "test@example.test", plan_type: "plus" },
  models: ["runtime-model"],
  default_model: "runtime-model",
};

function mount() {
  const onStatus = vi.fn();
  render(
    <React.StrictMode>
      <NextIntlClientProvider locale="en" messages={messages}>
        <CodexLogin onStatusChange={onStatus} />
      </NextIntlClientProvider>
    </React.StrictMode>,
  );
  return onStatus;
}

beforeEach(() => {
  vi.mocked(api.codexStatus).mockReset().mockResolvedValue(disconnected);
  vi.mocked(api.codexLogin).mockReset().mockResolvedValue(pending);
  vi.mocked(api.codexCancel)
    .mockReset()
    .mockResolvedValue({ ...disconnected, login: { login_id: "login-1", state: "cancelled" } });
  vi.mocked(api.codexLogout).mockReset().mockResolvedValue(disconnected);
});
afterEach(cleanup);

describe("Codex device login", () => {
  it("shows a device code and browser link, then clears them after authorization", async () => {
    vi.mocked(api.codexStatus)
      .mockResolvedValueOnce(disconnected)
      .mockResolvedValueOnce(disconnected)
      .mockResolvedValue(connected);
    const onStatus = mount();
    await waitFor(() => expect(screen.getByRole("button", { name: "Sign in with Codex" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Sign in with Codex" }));
    expect(await screen.findByText("TEST-CODE")).toBeVisible();
    expect(screen.getByRole("link", { name: "Open verification page" })).toHaveAttribute(
      "href",
      "https://auth.openai.com/codex/device",
    );
    await waitFor(() => expect(screen.getByText("Connected")).toBeVisible(), { timeout: 4000 });
    expect(screen.queryByText("TEST-CODE")).not.toBeInTheDocument();
    expect(screen.getByText("test@example.test")).toBeVisible();
    expect(onStatus).toHaveBeenLastCalledWith(connected);
  });

  it("cancels only the current login and allows requesting a new code", async () => {
    mount();
    await waitFor(() => expect(screen.getByRole("button", { name: "Sign in with Codex" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Sign in with Codex" }));
    await screen.findByText("TEST-CODE");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(api.codexCancel).toHaveBeenCalledWith("login-1"));
    await waitFor(() => expect(screen.queryByText("TEST-CODE")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Sign in with Codex" })).toBeEnabled();
  });

  it("does not overwrite a newly issued code with an older status response", async () => {
    let resolveOld: (status: CodexLoginStatus) => void = () => undefined;
    vi.mocked(api.codexStatus)
      .mockResolvedValueOnce(disconnected)
      .mockResolvedValueOnce(disconnected)
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve;
          }),
      );
    mount();
    await waitFor(() => expect(screen.getByRole("button", { name: "Sign in with Codex" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Refresh sign-in status" }));
    fireEvent.click(screen.getByRole("button", { name: "Sign in with Codex" }));
    await screen.findByText("TEST-CODE");
    await act(async () => resolveOld(disconnected));
    expect(screen.getByText("TEST-CODE")).toBeVisible();
  });

  it("disconnects a linked account and returns to the login action", async () => {
    vi.mocked(api.codexStatus).mockResolvedValue(connected);
    mount();
    fireEvent.click(await screen.findByRole("button", { name: "Disconnect" }));
    await waitFor(() => expect(api.codexLogout).toHaveBeenCalledOnce());
    await waitFor(() => expect(screen.getByRole("button", { name: "Sign in with Codex" })).toBeEnabled());
    expect(screen.queryByText("test@example.test")).not.toBeInTheDocument();
  });
});
