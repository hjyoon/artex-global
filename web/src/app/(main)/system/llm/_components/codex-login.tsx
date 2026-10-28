"use client";

import * as React from "react";

import {
  CheckCircle2Icon,
  CopyIcon,
  ExternalLinkIcon,
  Loader2Icon,
  LogInIcon,
  LogOutIcon,
  RefreshCwIcon,
} from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import type { CodexLoginStatus } from "@/lib/types";

export function CodexLogin({ onStatusChange }: { onStatusChange: (status: CodexLoginStatus) => void }) {
  const t = useTranslations("codexLogin");
  const headingId = React.useId();
  const mounted = React.useRef(false);
  const sequence = React.useRef(0);
  const inAction = React.useRef(false);
  const [status, setStatus] = React.useState<CodexLoginStatus | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [busy, setBusy] = React.useState(false);
  const pending = status?.login?.state === "pending" ? status.login : null;
  const endMessages = { pending: "", cancelled: t("cancelled"), expired: t("expired"), failed: t("loginFailed") };

  const applyStatus = React.useCallback(
    (value: CodexLoginStatus) => {
      if (!mounted.current) return;
      setStatus(value);
      onStatusChange(value);
    },
    [onStatusChange],
  );

  const refresh = React.useCallback(async () => {
    if (inAction.current) return;
    const ticket = ++sequence.current;
    setLoading(true);
    try {
      const value = await api.codexStatus();
      if (ticket === sequence.current) applyStatus(value);
    } catch {
      if (ticket === sequence.current) applyStatus({ available: false, connected: false });
    } finally {
      if (mounted.current && ticket === sequence.current) setLoading(false);
    }
  }, [applyStatus]);

  React.useEffect(() => {
    mounted.current = true;
    void refresh();
    return () => {
      mounted.current = false;
      sequence.current += 1;
    };
  }, [refresh]);

  React.useEffect(() => {
    if (!pending) return;
    const timer = setInterval(() => void refresh(), 2000);
    return () => clearInterval(timer);
  }, [pending, refresh]);

  async function action(run: () => Promise<CodexLoginStatus>) {
    if (inAction.current) return;
    inAction.current = true;
    const ticket = ++sequence.current;
    setBusy(true);
    try {
      const value = await run();
      if (ticket === sequence.current) applyStatus(value);
    } catch (error) {
      if (mounted.current) toast.error(t("actionFailed"), { description: (error as Error).message });
    } finally {
      inAction.current = false;
      if (mounted.current) setBusy(false);
    }
  }

  async function copyCode() {
    try {
      if (!pending?.user_code || !navigator.clipboard) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(pending.user_code);
      toast.success(t("copied"));
    } catch {
      toast.error(t("copyFailed"));
    }
  }

  return (
    <section aria-labelledby={headingId} className="grid gap-3 rounded-lg border p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-1">
          <h3 id={headingId} className="font-medium text-sm">
            {t("title")}
          </h3>
          <p className="text-muted-foreground text-xs leading-relaxed">{t("description")}</p>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("retry")}
          onClick={() => void refresh()}
          disabled={loading || busy}
        >
          <RefreshCwIcon className={loading ? "animate-spin" : ""} />
        </Button>
      </div>

      <div aria-live="polite" className="grid gap-3">
        {loading && !status && (
          <p className="flex items-center gap-2 text-muted-foreground text-xs">
            <Loader2Icon className="size-3 animate-spin" /> {t("checking")}
          </p>
        )}

        {status && !status.available && <p className="text-muted-foreground text-xs">{t("unavailable")}</p>}

        {status?.connected && (
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className="gap-1 border-emerald-500/50 text-emerald-600 dark:text-emerald-400">
              <CheckCircle2Icon className="size-3" /> {t("connected")}
            </Badge>
            {status.account?.email && <span className="break-all text-sm">{status.account.email}</span>}
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="ml-auto"
              onClick={() => void action(api.codexLogout)}
              disabled={busy}
            >
              <LogOutIcon /> {t("signOut")}
            </Button>
          </div>
        )}

        {pending && (
          <div className="grid gap-3 rounded-md bg-muted/50 p-3">
            <p className="text-muted-foreground text-xs">{t("codeLabel")}</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 select-all break-all font-mono font-semibold text-2xl tracking-wider">
                {pending.user_code}
              </code>
              <Button type="button" size="icon-sm" variant="outline" aria-label={t("copy")} onClick={copyCode}>
                <CopyIcon />
              </Button>
            </div>
            <div className="flex flex-wrap gap-2">
              {pending.verification_url && (
                <Button asChild size="sm">
                  <a href={pending.verification_url} target="_blank" rel="noopener noreferrer">
                    {t("openPage")} <ExternalLinkIcon />
                  </a>
                </Button>
              )}
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void action(() => api.codexCancel(pending.login_id))}
                disabled={busy}
              >
                {t("cancel")}
              </Button>
            </div>
            <p className="flex items-center gap-2 text-muted-foreground text-xs">
              <Loader2Icon className="size-3 animate-spin" /> {t("waiting")}
            </p>
          </div>
        )}

        {status?.login && status.login.state !== "pending" && (
          <p className="text-muted-foreground text-xs">{endMessages[status.login.state]}</p>
        )}

        {!status?.connected && !pending && (
          <Button
            type="button"
            variant="outline"
            className="justify-self-start"
            onClick={() => void action(api.codexLogin)}
            disabled={busy || !status?.available}
          >
            {busy ? <Loader2Icon className="animate-spin" /> : <LogInIcon />}
            {t("login")}
          </Button>
        )}
      </div>
    </section>
  );
}
