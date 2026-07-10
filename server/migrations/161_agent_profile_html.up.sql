-- Agent profile / persona HTML. Stores the agent-persona.html content from
-- an agentwaker role import (or a manually uploaded HTML document) so the
-- agent detail page can render a rich visual persona card in a sandboxed
-- iframe. NULL means "no profile configured". TEXT is unconstrained in size
-- because persona HTML can embed inline CSS / SVG / chart JS (observed
-- 17–40 KiB in practice).
ALTER TABLE agent ADD COLUMN profile_html TEXT;
