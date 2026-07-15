"use client";

import { Languages } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { cn } from "@multica/ui/lib/utils";

export type ContentLanguage = "en" | "zh";

export function LocalizedContentToggle({
  value,
  onChange,
  hasChinese,
  ariaLabel,
  className,
}: {
  value: ContentLanguage;
  onChange: (value: ContentLanguage) => void;
  hasChinese: boolean;
  ariaLabel: string;
  className?: string;
}) {
  if (!hasChinese) return null;

  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={cn(
        "inline-flex items-center gap-0.5 rounded-md border bg-muted/40 p-0.5",
        className,
      )}
    >
      <Languages className="mx-1.5 h-3.5 w-3.5 text-muted-foreground" />
      {(["zh", "en"] as const).map((language) => (
        <Button
          key={language}
          type="button"
          size="xs"
          variant={value === language ? "secondary" : "ghost"}
          aria-pressed={value === language}
          onClick={() => onChange(language)}
          className="h-6 px-2 text-[11px] shadow-none"
        >
          {language === "zh" ? "中文" : "English"}
        </Button>
      ))}
    </div>
  );
}
