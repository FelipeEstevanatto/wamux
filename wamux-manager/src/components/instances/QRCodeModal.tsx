/**
 * QRCode Modal Component
 * Displays QR code for WhatsApp connection
 */

import { useEffect, useRef, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Button,
} from '@/components/ui';
import {
  QrCode,
  RefreshCw,
  X,
  CheckCircle2,
  Loader2,
  AlertCircle,
} from 'lucide-react';
import { toast } from 'sonner';
import { useI18n } from '@/i18n/I18nContext';
import type { Instance, InstanceOverview } from '@/types/instance';

interface QRCodeModalProps {
  instance: Instance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onRefresh?: () => Promise<void>;
  /** Cheap status-only check used while waiting for the user to scan. */
  onCheckConnection?: () => Promise<void>;
  /** True while the backend is still generating the QR/pairing code. */
  isLoading?: boolean;
  /** Already-paired instance: the session is resuming and there is no QR to scan. */
  isPaired?: boolean;
  /** Connection/QR generation failure shown inline instead of a toast only. */
  error?: string | null;
  /** Live overview, used to show the real WhatsApp profile name on success. */
  overview?: InstanceOverview;
}

export default function QRCodeModal({
  instance,
  open,
  onOpenChange,
  onRefresh,
  onCheckConnection,
  isLoading = false,
  isPaired = false,
  error = null,
  overview,
}: QRCodeModalProps) {
  const { t } = useI18n();
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isChecking, setIsChecking] = useState(false);
  const checkInFlight = useRef(false);

  // Callbacks live in refs and are kept OUT of the effect dependencies on
  // purpose: they are recreated on every render, so having them in the list
  // re-ran the polling effect on each update, cancelling the timer and
  // restarting the countdown before it elapsed. `instance` is replaced by the
  // primitive `isConnected` for the same reason.
  const onCheckRef = useRef(onCheckConnection);
  useEffect(() => {
    onCheckRef.current = onCheckConnection;
  }, [onCheckConnection]);

  const isConnected = instance?.connected ?? false;

  // Poll quickly while the QR is on screen (2s instead of the old 10s), so the
  // success screen appears as soon as WhatsApp confirms the scan. The parent
  // checks a single instance, which is cheap.
  useEffect(() => {
    if (!open || isConnected) return;

    const check = () => {
      if (checkInFlight.current) return;
      const fn = onCheckRef.current;
      if (!fn) return;
      checkInFlight.current = true;
      fn()
        .catch((err) => {
          console.error('Connection check failed:', err);
        })
        .finally(() => {
          checkInFlight.current = false;
        });
    };

    const first = window.setTimeout(check, 800);
    const interval = window.setInterval(check, 2000);

    return () => {
      window.clearTimeout(first);
      window.clearInterval(interval);
    };
  }, [open, isConnected]);

  const handleRefresh = async () => {
    if (!onRefresh) return;

    setIsRefreshing(true);
    try {
      await onRefresh();
      toast.success(t('messages.qrUpdated'));
    } catch (error) {
      console.error('Erro ao atualizar QR Code:', error);
      toast.error(t('messages.qrUpdateError'));
    } finally {
      setIsRefreshing(false);
    }
  };

  const handleCheckNow = async () => {
    if (!onCheckConnection) return;

    setIsChecking(true);
    const startedAt = Date.now();
    try {
      await onCheckConnection();
    } catch (error) {
      console.error('Erro ao verificar conexão:', error);
    } finally {
      // Keep the spinner visible long enough to feel like a real action.
      const elapsed = Date.now() - startedAt;
      if (elapsed < 500) {
        await new Promise((resolve) => setTimeout(resolve, 500 - elapsed));
      }
      setIsChecking(false);
    }
  };

  if (!instance) return null;

  // If already connected, show success message
  if (instance.connected) {
    const profileName = overview?.profileName || instance.profileName;
    const showProfile =
      profileName && profileName !== instance.instanceName;

    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-green-500">
              <CheckCircle2 className="h-5 w-5" />
              {t('messages.connectedSuccessTitle')}
            </DialogTitle>
            <DialogDescription className="text-sidebar-foreground/70">
              {t('messages.connectedSuccessDesc', { name: instance.instanceName })}
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col items-center gap-4 py-6">
            <div className="rounded-full bg-green-500/10 p-4">
              <CheckCircle2 className="h-12 w-12 text-green-500" />
            </div>
            {showProfile && (
              <div className="flex items-center gap-3 text-center">
                {overview?.profilePicUrl && (
                  <img
                    src={overview.profilePicUrl}
                    alt={profileName}
                    className="h-10 w-10 rounded-full object-cover"
                    referrerPolicy="no-referrer"
                  />
                )}
                <div className="text-left">
                  <p className="text-sm text-sidebar-foreground/60">
                    {t('messages.connectedAs')}
                  </p>
                  <p className="text-lg font-semibold text-sidebar-foreground">
                    {profileName}
                  </p>
                </div>
              </div>
            )}
          </div>

          <div className="flex justify-end">
            <Button
              onClick={() => onOpenChange(false)}
              className="w-full sm:w-auto"
            >
              {t('common.close')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    );
  }

  const hasQr = Boolean(instance.qrcode?.base64);
  const hasPairingCode = Boolean(instance.qrcode?.pairingCode);

  // Show QR Code
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <QrCode className="h-5 w-5 text-primary" />
            {t('messages.connectTitle')}
          </DialogTitle>
          <DialogDescription className="text-sidebar-foreground/70">
            {isPaired
              ? t('messages.reconnectingPairedHint')
              : isLoading && !hasQr
                ? `${t('messages.generatingQr')} ${t('messages.generatingQrHint')}`
                : (
                    <>
                      {t('messages.scanPrefix')}
                      <strong>{instance.instanceName}</strong>
                    </>
                  )}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* QR Code Display */}
          <div className="flex flex-col items-center gap-4">
            {hasQr ? (
              <div className="rounded-lg border-2 border-sidebar-border bg-white p-4">
                <img
                  src={instance.qrcode?.base64}
                  alt={t('messages.qrAlt')}
                  className="h-64 w-64"
                />
              </div>
            ) : error ? (
              <div className="flex h-64 w-64 items-center justify-center rounded-lg border-2 border-dashed border-destructive/40 bg-destructive/5 p-6">
                <div className="text-center">
                  <AlertCircle className="mx-auto h-12 w-12 text-destructive" />
                  <p className="mt-3 text-sm font-medium text-foreground">
                    {t('messages.qrErrorTitle')}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">{error}</p>
                </div>
              </div>
            ) : isPaired ? (
              <div className="flex h-64 w-64 items-center justify-center rounded-lg border-2 border-dashed border-sidebar-border bg-sidebar">
                <div className="text-center">
                  <Loader2 className="mx-auto h-12 w-12 animate-spin text-primary" />
                  <p className="mt-3 text-sm font-medium text-sidebar-foreground">
                    {t('messages.reconnectingPaired')}
                  </p>
                  <p className="mt-1 text-xs text-sidebar-foreground/60">
                    {t('messages.reconnectingPairedHint')}
                  </p>
                </div>
              </div>
            ) : isLoading ? (
              <div className="flex h-64 w-64 items-center justify-center rounded-lg border-2 border-dashed border-sidebar-border bg-sidebar">
                <div className="text-center">
                  <Loader2 className="mx-auto h-12 w-12 animate-spin text-primary" />
                  <p className="mt-3 text-sm font-medium text-sidebar-foreground">
                    {t('messages.generatingQr')}
                  </p>
                  <p className="mt-1 text-xs text-sidebar-foreground/60">
                    {t('messages.generatingQrHint')}
                  </p>
                </div>
              </div>
            ) : !hasPairingCode ? (
              <div className="flex h-64 w-64 items-center justify-center rounded-lg border-2 border-dashed border-sidebar-border bg-sidebar">
                <div className="text-center">
                  <QrCode className="mx-auto h-12 w-12 text-sidebar-foreground/40" />
                  <p className="mt-2 text-sm text-sidebar-foreground/60">
                    {t('messages.awaitingQr')}
                  </p>
                </div>
              </div>
            ) : null}

            {/* Pairing Code (if available) */}
            {hasPairingCode && (
              <div className="w-full rounded-lg bg-sidebar-accent p-3 text-center">
                <p className="text-xs text-sidebar-foreground/60">
                  {t('messages.pairingCode')}
                </p>
                <p className="mt-1 font-mono text-lg font-semibold text-sidebar-foreground">
                  {instance.qrcode?.pairingCode}
                </p>
              </div>
            )}

            {/* Live status: makes it obvious the modal is watching the scan. */}
            {hasQr && !isLoading && !error && (
              <div className="flex items-center justify-center gap-2 text-xs text-sidebar-foreground/70">
                <span className="relative flex h-2 w-2">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-75" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
                </span>
                <span>{t('messages.waitingScan')}</span>
                <span className="text-sidebar-foreground/40">
                  · {t('messages.autoChecking')}
                </span>
              </div>
            )}
          </div>

          {/* Instructions */}
          {hasQr && !error && (
            <div className="rounded-lg bg-sidebar-accent p-4">
              <p className="text-sm font-medium text-sidebar-foreground">
                {t('messages.howToConnect')}
              </p>
              <ol className="mt-2 space-y-1 text-sm text-sidebar-foreground/70">
                <li>{t('messages.step1')}</li>
                <li>{t('messages.step2')}</li>
                <li>{t('messages.step3')}</li>
                <li>{t('messages.step4')}</li>
                <li>{t('messages.step5')}</li>
              </ol>
            </div>
          )}

          {/* Actions */}
          <div className="flex flex-col gap-2">
            {(hasQr || isPaired) && onCheckConnection && (
              <Button
                onClick={handleCheckNow}
                disabled={isChecking || isLoading}
                className="w-full"
              >
                {isChecking ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    {t('messages.checkingConnection')}
                  </>
                ) : (
                  <>
                    <CheckCircle2 className="mr-2 h-4 w-4" />
                    {isPaired
                      ? t('messages.checkConnection')
                      : t('messages.checkNow')}
                  </>
                )}
              </Button>
            )}

            <div className="flex gap-2">
              {!isPaired && (
                <Button
                  variant="outline"
                  onClick={handleRefresh}
                  disabled={isRefreshing || isLoading}
                  className="flex-1 bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent"
                >
                  {isRefreshing ? (
                    <>
                      <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                      {t('messages.updating')}
                    </>
                  ) : (
                    <>
                      <RefreshCw className="mr-2 h-4 w-4" />
                      {t('messages.updateQr')}
                    </>
                  )}
                </Button>
              )}
              <Button
                variant="outline"
                onClick={() => onOpenChange(false)}
                className="bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
