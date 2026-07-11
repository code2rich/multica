"use client";

import type { ReactNode } from "react";
import { DashboardGuard } from "./dashboard-guard";
import { NavigationProgress } from "./navigation-progress";
import { WorkspacePresencePrefetch } from "./workspace-presence-prefetch";
import { ModalRegistry } from "../modals/registry";
import { SourceBackfillModal } from "../onboarding";
import { TopNav } from "./top-nav";

interface TopNavLayoutProps {
  children: ReactNode;
  /** Absolute-positioned overlays (e.g. ChatWindow, ChatFab) */
  extra?: ReactNode;
  /** Loading indicator */
  loadingIndicator?: ReactNode;
}

export function TopNavLayout({ children, extra, loadingIndicator }: TopNavLayoutProps) {
  return (
    <DashboardGuard
      loadingFallback={
        <div className="flex h-svh items-center justify-center">
          {loadingIndicator}
        </div>
      }
    >
      <div className="flex h-svh flex-col bg-background">
        <WorkspacePresencePrefetch />
        <TopNav />
        <main className="relative flex flex-1 flex-col overflow-hidden">
          <NavigationProgress />
          {children}
          <ModalRegistry />
          <SourceBackfillModal />
          {extra}
        </main>
      </div>
    </DashboardGuard>
  );
}
