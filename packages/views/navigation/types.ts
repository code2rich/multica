export interface NavigationAdapter {
  push(path: string): void;
  replace(path: string): void;
  back(): void;
  pathname: string;
  searchParams: URLSearchParams;
  /**
   * Optional handler that opens a path in a new tab. Optional `title` overrides
   * the default tab label. `opts.activate` controls focus:
   *   - `false` / omitted → background tab (browser cmd+click semantics; what
   *     modifier-click on links and mentions should use).
   *   - `true` → foreground tab (explicit "Open in new tab" toolbar buttons,
   *     where the user is asking to move into the new context).
   * Cross-workspace paths always switch workspace, regardless of `activate`.
   */
  openInNewTab?: (
    path: string,
    title?: string,
    opts?: { activate?: boolean },
  ) => void;
  /** Return a shareable URL for a path. On web this is `origin + path`. */
  getShareableUrl: (path: string) => string;
  /**
   * Optional: warm up route assets / RSC payload for a path. Web wires this
   * to `router.prefetch`. Callers must invoke via `prefetch?.(href)`.
   */
  prefetch?: (path: string) => void;
}
