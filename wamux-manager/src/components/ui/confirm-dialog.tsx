import { useRef, type ReactNode } from "react";

import { Button } from "./button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./dialog";

interface ConfirmDialogProps {
  open: boolean;
  title: ReactNode;
  description: ReactNode;
  confirmLabel: ReactNode;
  cancelLabel: ReactNode;
  destructive?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * A themed replacement for window.confirm. Cancel comes first, so it takes the
 * initial focus and pressing Enter straight away never confirms.
 */
function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel,
  destructive = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  // Radix only restores focus to a DialogTrigger, and this dialog has none.
  const returnFocusTo = useRef<HTMLElement | null>(null);

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onCancel()}>
      <DialogContent
        role="alertdialog"
        showCloseButton={false}
        className="sm:max-w-md"
        onOpenAutoFocus={() => {
          returnFocusTo.current = document.activeElement as HTMLElement | null;
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          returnFocusTo.current?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            {cancelLabel}
          </Button>
          <Button variant={destructive ? "destructive" : "default"} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export { ConfirmDialog };
