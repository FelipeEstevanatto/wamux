/**
 * Local UI primitives.
 *
 * Plain, editable source (shadcn/ui-style, built on Radix UI + Tailwind v4) so
 * the manager is fully self-contained: no external UI package dependency. Only
 * the primitives this app uses are included — add a new one here and re-export
 * it when needed.
 *
 * The theme tokens live in `src/styles/globals.css`.
 */
export * from "./alert";
export * from "./badge";
export * from "./button";
export * from "./card";
export * from "./dialog";
export * from "./dropdown-menu";
export * from "./input";
export * from "./label";
export * from "./sheet";
export * from "./skeleton";
