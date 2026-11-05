import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from './confirm-dialog';

// React 19 requires this flag for act() outside the test runner's own setup.
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

const button = (name: string) =>
  [...document.querySelectorAll('button')].find((b) => b.textContent === name) as HTMLButtonElement;

describe('ConfirmDialog', () => {
  let root: Root;
  const onConfirm = vi.fn();
  const onCancel = vi.fn();

  const render = (open: boolean) =>
    act(() => {
      root.render(
        <ConfirmDialog
          open={open}
          title="Delete"
          description="Cannot be undone"
          confirmLabel="Yes"
          cancelLabel="No"
          destructive
          onConfirm={onConfirm}
          onCancel={onCancel}
        />
      );
    });

  beforeEach(() => {
    onConfirm.mockClear();
    onCancel.mockClear();
    const host = document.createElement('div');
    document.body.appendChild(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    document.body.innerHTML = '';
  });

  it('renders nothing while closed', () => {
    render(false);
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('is an alert dialog that focuses Cancel first', () => {
    render(true);
    expect(document.querySelector('[role="alertdialog"]')?.textContent).toContain(
      'Cannot be undone'
    );
    expect(document.activeElement).toBe(button('No'));
  });

  it('returns focus to what opened it', async () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();

    render(true);
    expect(document.activeElement).not.toBe(opener);

    render(false);
    await act(() => new Promise((resolve) => setTimeout(resolve, 0)));
    expect(document.activeElement).toBe(opener);
  });

  it('confirms only from the confirm button', () => {
    render(true);
    act(() => button('Yes').click());
    expect(onConfirm).toHaveBeenCalledOnce();
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('cancels from the cancel button', () => {
    render(true);
    act(() => button('No').click());
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('cancels on Escape', () => {
    render(true);
    act(() => {
      document.activeElement?.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
      );
    });
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
