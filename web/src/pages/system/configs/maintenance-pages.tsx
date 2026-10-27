import { useEffect, useState } from "react";
import { weighingApi } from "@/api/weighing";
import { PageHeader } from "@/components/common/page-header";
import { LocalIdentityConfiguration } from "./local-identity-configuration";
import { TicketSequenceConfiguration } from "./ticket-sequence-configuration";
import { TicketTitleConfiguration } from "./ticket-title-configuration";

export function LocalIdentityPage() {
  return <><PageHeader title="本厂身份" /><LocalIdentityConfiguration /></>;
}

export function TicketSequencePage() {
  return <><PageHeader title="流水号水位" /><TicketSequenceConfiguration /></>;
}

export function TicketTitlePage() {
  const [allowed, setAllowed] = useState<boolean | null>(null);
  useEffect(() => { void weighingApi.capabilities().then(c => setAllowed(c.permissionCodes.includes("system:config"))).catch(() => setAllowed(false)); }, []);
  return <><PageHeader title="过磅单标题" />{allowed === null ? <p role="status">正在检查权限…</p> : allowed ? <TicketTitleConfiguration /> : <p role="alert">无权访问过磅单标题</p>}</>;
}
