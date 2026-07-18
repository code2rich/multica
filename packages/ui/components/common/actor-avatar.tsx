"use client";

import { useState, useEffect } from "react";
import { Users } from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import {
  AVATAR_SIZE_PX,
  DEFAULT_AVATAR_SIZE,
  type AvatarSize,
} from "@multica/ui/lib/avatar-size";
import {
  isAgentIconUrl,
  agentIconKeyFromUrl,
  defaultAgentIconKey,
} from "@multica/ui/lib/agent-icon-url";
import { AgentIcon } from "./agent-icons";
import { MulticaIcon } from "./multica-icon";

interface ActorAvatarProps {
  name: string;
  initials: string;
  avatarUrl?: string | null;
  isAgent?: boolean;
  isSystem?: boolean;
  isSquad?: boolean;
  size?: AvatarSize;
  className?: string;
}

function ActorAvatar({
  name,
  initials,
  avatarUrl,
  isAgent,
  isSystem,
  isSquad,
  size = DEFAULT_AVATAR_SIZE,
  className,
}: ActorAvatarProps) {
  const [imgError, setImgError] = useState(false);
  const px = AVATAR_SIZE_PX[size];

  useEffect(() => {
    setImgError(false);
  }, [avatarUrl]);

  // Every actor — member, agent, squad, or system — renders as a circle. This
  // is the single source of truth for avatar shape; the upload editors mirror
  // it (packages/views/common/avatar-upload-control.tsx).
  return (
    <div
      data-slot="avatar"
      className={cn(
        "inline-flex shrink-0 items-center justify-center font-medium overflow-hidden",
        (!avatarUrl || imgError) && "bg-muted text-muted-foreground",
        className,
        // rounded-full stays last so a call-site `className` can never override
        // the circle — avatar shape is a hard invariant, not a per-site choice.
        "rounded-full"
      )}
      style={{ width: px, height: px, fontSize: px * 0.45 }}
    >
      {avatarUrl && isAgentIconUrl(avatarUrl) ? (
        // Built-in icon reference: render the matching SVG. An unknown key
        // (corrupt value) falls through to the name-derived default so an
        // agent always shows a real icon.
        <AgentIcon
          iconKey={agentIconKeyFromUrl(avatarUrl) ?? defaultAgentIconKey(name)}
          size={px}
        />
      ) : avatarUrl && !imgError ? (
        <img
          src={avatarUrl}
          alt={name}
          className="h-full w-full object-cover"
          onError={() => setImgError(true)}
        />
      ) : isSystem ? (
        <MulticaIcon noSpin style={{ width: px * 0.55, height: px * 0.55 }} />
      ) : isAgent ? (
        // No uploaded avatar: derive a stable icon from the name so every
        // agent — including pre-existing ones with a null avatar_url — gets a
        // distinct identity with zero backfill.
        <AgentIcon iconKey={defaultAgentIconKey(name)} size={px} />
      ) : isSquad ? (
        <Users style={{ width: px * 0.55, height: px * 0.55 }} />
      ) : (
        initials
      )}
    </div>
  );
}

export { ActorAvatar, type ActorAvatarProps };
