"use client";

import { TopNavLayout } from "@multica/views/layout";
import { MulticaIcon } from "@multica/ui/components/common/multica-icon";
import { SearchCommand } from "@multica/views/search";
import { FloatingChat } from "@multica/views/chat";
import { WebNotificationBridge } from "@/components/web-notification-bridge";

export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <TopNavLayout
      loadingIndicator={<MulticaIcon className="size-6" />}
      extra={
        <>
          <SearchCommand />
          <WebNotificationBridge />
          <FloatingChat />
        </>
      }
    >
      {children}
    </TopNavLayout>
  );
}
