"use client";

import React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Inbox,
  MessageSquare,
  ListTodo,
  Bot,
  Monitor,
  ChevronDown,
  Settings,
  LogOut,
  Plus,
  Check,
  BookOpenText,
  SquarePen,
  CircleUser,
  FolderKanban,
  BarChart3,
  Zap,
  Users,
  Search,
  User,
  Menu,
} from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@multica/ui/components/ui/sheet";
import { ActorAvatar } from "@multica/ui/components/common/actor-avatar";
import { WorkspaceAvatar } from "../workspace/workspace-avatar";
import { AppLink, useNavigation } from "../navigation";
import { useLogout } from "../auth";
import { useT } from "../i18n";
import { useAuthStore } from "@multica/core/auth";
import { useCurrentWorkspace, useWorkspacePaths, paths } from "@multica/core/paths";
import {
  workspaceListOptions,
  myInvitationListOptions,
  workspaceKeys,
} from "@multica/core/workspace/queries";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { inboxKeys, deduplicateInboxItems, inboxUnreadSummaryOptions, hasOtherWorkspaceUnread, unreadWorkspaceIds } from "@multica/core/inbox/queries";
import { chatSessionsOptions } from "@multica/core/chat/queries";
import { api } from "@multica/core/api";
import { useConfigStore } from "@multica/core/config";
import { useModalStore } from "@multica/core/modals";
import { openCreateIssueWithPreference } from "@multica/core/issues/stores/create-mode-store";
import { useSearchStore } from "../search/search-store";
import { isMac, modKey } from "@multica/core/platform";

type NavKey =
  | "inbox"
  | "chat"
  | "myIssues"
  | "issues"
  | "projects"
  | "autopilots"
  | "agents"
  | "squads"
  | "usage"
  | "runtimes"
  | "skills"
  | "settings";

type NavLabelKey =
  | "inbox"
  | "chat"
  | "my_issues"
  | "issues"
  | "projects"
  | "autopilots"
  | "agents"
  | "squads"
  | "usage"
  | "runtimes"
  | "skills"
  | "settings";

const EMPTY_WORKSPACES: Awaited<ReturnType<typeof api.listWorkspaces>> = [];
const EMPTY_INVITATIONS: Awaited<ReturnType<typeof api.listMyInvitations>> = [];
const EMPTY_INBOX: Awaited<ReturnType<typeof api.listInbox>> = [];
const EMPTY_INBOX_SUMMARY: Awaited<ReturnType<typeof api.getInboxUnreadSummary>> = [];

function isNavActive(pathname: string, href: string): boolean {
  return pathname === href || pathname.startsWith(href + "/");
}

function NavLink({
  href,
  icon: Icon,
  label,
  isActive,
  badge,
}: {
  href: string;
  icon: React.ComponentType<{ className?: string }>;
  label: React.ReactNode;
  isActive: boolean;
  badge?: React.ReactNode;
}) {
  return (
    <AppLink
      href={href}
      className={cn(
        "flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm font-medium transition-colors",
        isActive
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-accent/70 hover:text-foreground"
      )}
    >
      <Icon className="size-4 shrink-0" />
      <span className="whitespace-nowrap">{label}</span>
      {badge}
    </AppLink>
  );
}

function Badge({ children }: { children: React.ReactNode }) {
  return (
    <span className="ml-0.5 rounded-full bg-primary px-1.5 py-0 text-[10px] font-medium text-primary-foreground">
      {children}
    </span>
  );
}

function IssuesNav({ p, pathname }: { p: ReturnType<typeof useWorkspacePaths>; pathname: string }) {
  const { t } = useT("layout");
  const myIssuesHref = p.myIssues();
  const allIssuesHref = p.issues();
  const isMyIssuesActive = isNavActive(pathname, myIssuesHref);
  const isAllIssuesActive = isNavActive(pathname, allIssuesHref);
  const isActive = isMyIssuesActive || isAllIssuesActive;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            className={cn(
              "flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm font-medium transition-colors",
              isActive
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/70 hover:text-foreground"
            )}
          >
            <ListTodo className="size-4 shrink-0" />
            <span className="whitespace-nowrap">
              {isAllIssuesActive ? t(($) => $.nav.issues) : t(($) => $.nav.my_issues)}
            </span>
            <ChevronDown className="size-3 shrink-0 opacity-60" />
          </button>
        }
      />
      <DropdownMenuContent align="start" sideOffset={4}>
        <DropdownMenuItem
          render={
            <AppLink href={myIssuesHref} className={cn(isMyIssuesActive && "bg-accent")} />
          }
        >
          <CircleUser className="size-4" />
          {t(($) => $.nav.my_issues)}
        </DropdownMenuItem>
        <DropdownMenuItem
          render={
            <AppLink href={allIssuesHref} className={cn(isAllIssuesActive && "bg-accent")} />
          }
        >
          <ListTodo className="size-4" />
          {t(($) => $.nav.issues)}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function WorkspaceSwitcher() {
  const { t } = useT("layout");
  const { push } = useNavigation();
  const workspace = useCurrentWorkspace();
  const user = useAuthStore((s) => s.user);
  const { data: workspaces = EMPTY_WORKSPACES } = useQuery(workspaceListOptions());
  const { data: myInvitations = EMPTY_INVITATIONS } = useQuery(myInvitationListOptions());
  const workspaceCreationDisabled = useConfigStore((s) => s.workspaceCreationDisabled);
  const queryClient = useQueryClient();

  const unreadSummary = useQuery({ ...inboxUnreadSummaryOptions(), enabled: !!workspace?.id }).data ?? EMPTY_INBOX_SUMMARY;
  const otherWorkspaceUnread = hasOtherWorkspaceUnread(unreadSummary, workspace?.id);
  const unreadWsIds = React.useMemo(() => unreadWorkspaceIds(unreadSummary), [unreadSummary]);

  const acceptInvitationMut = useMutation({
    mutationFn: (id: string) => api.acceptInvitation(id),
    onSuccess: async (_, invitationId) => {
      const invitation = myInvitations.find((i) => i.id === invitationId);
      queryClient.invalidateQueries({ queryKey: workspaceKeys.myInvitations() });
      const list = await queryClient.fetchQuery({ ...workspaceListOptions(), staleTime: 0 });
      const joined = invitation ? list.find((w) => w.id === invitation.workspace_id) : null;
      if (joined) {
        push(paths.workspace(joined.slug).issues());
      }
    },
  });

  const declineInvitationMut = useMutation({
    mutationFn: (id: string) => api.declineInvitation(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workspaceKeys.myInvitations() });
    },
  });

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm font-medium transition-colors hover:bg-accent"
          >
            <span className="relative">
              <WorkspaceAvatar name={workspace?.name ?? "M"} avatarUrl={workspace?.avatar_url} size="sm" />
              {(myInvitations.length > 0 || otherWorkspaceUnread) && (
                <span className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-brand ring-1 ring-background" />
              )}
            </span>
            <span className="max-w-[140px] truncate hidden sm:block">{workspace?.name ?? "Multica"}</span>
            <ChevronDown className="size-3 opacity-60 hidden sm:block" />
          </button>
        }
      />
      <DropdownMenuContent className="w-auto min-w-56" align="start" side="bottom" sideOffset={4}>
        <div className="flex items-center gap-2.5 px-2 py-1.5">
          <ActorAvatar
            name={user?.name ?? ""}
            initials={(user?.name ?? "U").charAt(0).toUpperCase()}
            avatarUrl={resolvePublicFileUrl(user?.avatar_url)}
            size="lg"
          />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium leading-tight">{user?.name}</p>
            <p className="truncate text-xs text-muted-foreground leading-tight">{user?.email}</p>
          </div>
        </div>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuLabel className="text-xs text-muted-foreground">{t(($) => $.sidebar.workspaces_label)}</DropdownMenuLabel>
          {workspaces.map((ws) => (
            <DropdownMenuItem
              key={ws.id}
              render={
                <AppLink href={paths.workspace(ws.slug).issues()} className="flex items-center gap-2" />
              }
            >
              <WorkspaceAvatar name={ws.name} avatarUrl={ws.avatar_url} size="sm" />
              <span className="flex-1 truncate">{ws.name}</span>
              {ws.id !== workspace?.id && unreadWsIds.has(ws.id) && <span className="size-2 rounded-full bg-brand" />}
              {ws.id === workspace?.id && <Check className="h-3.5 w-3.5 text-primary" />}
            </DropdownMenuItem>
          ))}
          {!workspaceCreationDisabled && (
            <DropdownMenuItem onClick={() => useModalStore.getState().open("create-workspace")}>
              <Plus className="h-3.5 w-3.5" />
              {t(($) => $.sidebar.create_workspace)}
            </DropdownMenuItem>
          )}
        </DropdownMenuGroup>
        {myInvitations.length > 0 && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs text-muted-foreground">{t(($) => $.sidebar.pending_invitations_label)}</DropdownMenuLabel>
              {myInvitations.map((inv) => (
                <div key={inv.id} className="flex items-center gap-2 px-2 py-1.5">
                  <WorkspaceAvatar name={inv.workspace_name ?? "W"} size="sm" />
                  <span className="flex-1 truncate text-sm">{inv.workspace_name ?? t(($) => $.sidebar.invitation_workspace_fallback)}</span>
                  <button
                    type="button"
                    className="text-xs px-2 py-0.5 rounded bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                    disabled={acceptInvitationMut.isPending}
                    onClick={(e) => {
                      e.stopPropagation();
                      acceptInvitationMut.mutate(inv.id);
                    }}
                  >
                    {t(($) => $.sidebar.invitation_join)}
                  </button>
                  <button
                    type="button"
                    className="text-xs px-2 py-0.5 rounded bg-muted text-muted-foreground hover:bg-muted/80 disabled:opacity-50"
                    disabled={declineInvitationMut.isPending}
                    onClick={(e) => {
                      e.stopPropagation();
                      declineInvitationMut.mutate(inv.id);
                    }}
                  >
                    {t(($) => $.sidebar.invitation_decline)}
                  </button>
                </div>
              ))}
            </DropdownMenuGroup>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function UserMenu({ p }: { p: ReturnType<typeof useWorkspacePaths> }) {
  const { t } = useT("layout");
  const user = useAuthStore((s) => s.user);
  const logout = useLogout();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm font-medium transition-colors hover:bg-accent"
          >
            <ActorAvatar
              name={user?.name ?? ""}
              initials={(user?.name ?? "U").charAt(0).toUpperCase()}
              avatarUrl={resolvePublicFileUrl(user?.avatar_url)}
              size="sm"
            />
            <span className="max-w-[100px] truncate hidden sm:block">{user?.name}</span>
            <ChevronDown className="size-3 opacity-60 hidden sm:block" />
          </button>
        }
      />
      <DropdownMenuContent align="end" side="bottom" sideOffset={4} className="w-56">
        <div className="flex items-center gap-2.5 px-2 py-1.5">
          <ActorAvatar
            name={user?.name ?? ""}
            initials={(user?.name ?? "U").charAt(0).toUpperCase()}
            avatarUrl={resolvePublicFileUrl(user?.avatar_url)}
            size="lg"
          />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium leading-tight">{user?.name}</p>
            <p className="truncate text-xs text-muted-foreground leading-tight">{user?.email}</p>
          </div>
        </div>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem
            render={
              <AppLink href={p.settings()} className="flex items-center gap-2" />
            }
          >
            <User className="size-4" />
            {t(($) => $.user_menu.account_settings)}
          </DropdownMenuItem>
          <DropdownMenuItem
            render={
              <AppLink href={p.usage()} className="flex items-center gap-2" />
            }
          >
            <BarChart3 className="size-4" />
            {t(($) => $.nav.usage)}
          </DropdownMenuItem>
          <DropdownMenuItem
            render={
              <AppLink href={p.runtimes()} className="flex items-center gap-2" />
            }
          >
            <Monitor className="size-4" />
            {t(($) => $.nav.runtimes)}
          </DropdownMenuItem>
          <DropdownMenuItem
            render={
              <AppLink href={p.settings()} className="flex items-center gap-2" />
            }
          >
            <Settings className="size-4" />
            {t(($) => $.nav.settings)}
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem variant="destructive" onClick={logout} className="flex items-center gap-2">
            <LogOut className="size-4" />
            {t(($) => $.sidebar.log_out)}
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SearchTriggerButton() {
  const { t } = useT("search");
  const open = () => useSearchStore.getState().setOpen(true);

  return (
    <Button variant="outline" size="sm" className="h-8 gap-1.5 text-muted-foreground" onClick={open}>
      <Search className="size-4" />
      <span className="hidden sm:inline">{t(($) => $.trigger.label)}</span>
      <kbd className="pointer-events-none hidden sm:inline-flex h-5 select-none items-center gap-0.5 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground">
        {isMac ? (
          <>
            <span className="text-xs">{modKey}</span>K
          </>
        ) : (
          `${modKey} K`
        )}
      </kbd>
    </Button>
  );
}

function NewIssueButton() {
  const { t } = useT("layout");
  return (
    <Button size="sm" className="h-8 gap-1.5" onClick={() => openCreateIssueWithPreference()}>
      <SquarePen className="size-4" />
      <span className="hidden sm:inline">{t(($) => $.sidebar.new_issue)}</span>
    </Button>
  );
}

function MobileNav({ p, pathname }: { p: ReturnType<typeof useWorkspacePaths>; pathname: string }) {
  const { t } = useT("layout");
  const [open, setOpen] = React.useState(false);
  const title = t(($) => $.mobile_nav.title);

  const centerNav: { key: NavKey; labelKey: NavLabelKey; icon: typeof FolderKanban }[] = [
    { key: "projects", labelKey: "projects", icon: FolderKanban },
    { key: "autopilots", labelKey: "autopilots", icon: Zap },
    { key: "agents", labelKey: "agents", icon: Bot },
    { key: "squads", labelKey: "squads", icon: Users },
    { key: "skills", labelKey: "skills", icon: BookOpenText },
  ];

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger
        render={
          <Button variant="ghost" size="icon-sm" className="lg:hidden">
            <Menu className="size-4" />
          </Button>
        }
      />
      <SheetContent side="left" className="w-64 p-4">
        <SheetHeader className="sr-only">
          <SheetTitle>{title}</SheetTitle>
        </SheetHeader>
        <div className="flex flex-col gap-1 pt-4">
          {centerNav.map((item) => {
            const href = p[item.key]();
            const isActive = isNavActive(pathname, href);
            return (
              <AppLink
                key={item.key}
                href={href}
                onClick={() => setOpen(false)}
                className={cn(
                  "flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium",
                  isActive ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-accent/70"
                )}
              >
                <item.icon className="size-4" />
                {t(($) => $.nav[item.labelKey])}
              </AppLink>
            );
          })}
        </div>
      </SheetContent>
    </Sheet>
  );
}

export function TopNav() {
  const { t } = useT("layout");
  const { pathname } = useNavigation();
  const p = useWorkspacePaths();
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id;

  const { data: inboxItems = EMPTY_INBOX } = useQuery({
    queryKey: wsId ? inboxKeys.list(wsId) : ["inbox", "disabled"],
    queryFn: () => api.listInbox(),
    enabled: !!wsId,
  });
  const unreadCount = React.useMemo(() => deduplicateInboxItems(inboxItems).filter((i) => !i.read).length, [inboxItems]);

  const { data: chatSessions = [] } = useQuery({
    ...chatSessionsOptions(wsId ?? ""),
    enabled: !!wsId,
  });
  const chatUnreadCount = React.useMemo(() => chatSessions.reduce((sum, s) => sum + (s.unread_count ?? 0), 0), [chatSessions]);

  const leftNav: { key: NavKey; icon: typeof Inbox; labelKey: NavLabelKey }[] = [
    { key: "inbox", icon: Inbox, labelKey: "inbox" },
    { key: "chat", icon: MessageSquare, labelKey: "chat" },
  ];

  const centerNav: { key: NavKey; icon: typeof FolderKanban; labelKey: NavLabelKey }[] = [
    { key: "projects", icon: FolderKanban, labelKey: "projects" },
    { key: "autopilots", icon: Zap, labelKey: "autopilots" },
    { key: "agents", icon: Bot, labelKey: "agents" },
    { key: "squads", icon: Users, labelKey: "squads" },
    { key: "skills", icon: BookOpenText, labelKey: "skills" },
  ];

  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b bg-background px-3">
      <WorkspaceSwitcher />

      <div className="flex items-center gap-1">
        {leftNav.map((item) => {
          const href = p[item.key]();
          const isActive = isNavActive(pathname, href);
          const badge = item.key === "inbox" && unreadCount > 0 ? <Badge>{unreadCount > 99 ? "99+" : unreadCount}</Badge> : undefined;
          const chatBadge = item.key === "chat" && chatUnreadCount > 0 ? <Badge>{chatUnreadCount > 99 ? "99+" : chatUnreadCount}</Badge> : undefined;
          return (
            <NavLink
              key={item.key}
              href={href}
              icon={item.icon}
              label={t(($) => $.nav[item.labelKey])}
              isActive={isActive}
              badge={badge ?? chatBadge}
            />
          );
        })}
        <IssuesNav p={p} pathname={pathname} />
      </div>

      <nav className="hidden lg:flex items-center gap-1">
        {centerNav.map((item) => {
          const href = p[item.key]();
          const isActive = isNavActive(pathname, href);
          return (
            <NavLink
              key={item.key}
              href={href}
              icon={item.icon}
              label={t(($) => $.nav[item.labelKey])}
              isActive={isActive}
            />
          );
        })}
      </nav>

      <MobileNav p={p} pathname={pathname} />

      <div className="ml-auto flex items-center gap-2">
        <SearchTriggerButton />
        <NewIssueButton />
        <UserMenu p={p} />
      </div>
    </header>
  );
}
