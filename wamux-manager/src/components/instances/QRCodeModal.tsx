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
import { QrCode, RefreshCw, X, CheckCircle2 } from 'lucide-react';
import { toast } from 'sonner';
import { useI18n } from '@/i18n/I18nContext';
import type { Instance } from '@/types/instance';

interface QRCodeModalProps {
  instance: Instance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onRefresh?: () => Promise<void>;
}

export default function QRCodeModal({
  instance,
  open,
  onOpenChange,
  onRefresh,
}: QRCodeModalProps) {
  const { t } = useI18n();
  const [isRefreshing, setIsRefreshing] = useState(false);

  // onRefresh lives in a ref and is kept OUT of the effect dependencies on
  // purpose: it is recreated on every render (it depends on the instance it
  // itself updates), so having it in the list re-ran the effect on each update,
  // cancelling the timer and restarting the countdown before the 10s elapsed.
  // The automatic refresh simply never fired. `instance` is replaced by the
  // primitive `isConnected` for the same reason.
  const onRefreshRef = useRef(onRefresh);
  useEffect(() => {
    onRefreshRef.current = onRefresh;
  }, [onRefresh]);

  const isConnected = instance?.connected ?? false;

  // Auto-refresh every 10 seconds to check connection status and update QR code
  useEffect(() => {
    if (!open || isConnected) return;

    const interval = setInterval(() => {
      onRefreshRef.current?.().catch((err) => {
        console.error('Auto-refresh failed:', err);
      });
    }, 10000); // 10 seconds (faster polling to detect connection)

    return () => clearInterval(interval);
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

  if (!instance) return null;

  // If already connected, show success message
  if (instance.connected) {
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
            {instance.profileName && (
              <div className="text-center">
                <p className="text-sm text-sidebar-foreground/60">
                  {t('messages.connectedAs')}
                </p>
                <p className="text-lg font-semibold text-sidebar-foreground">
                  {instance.profileName}
                </p>
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
            {t('messages.scanPrefix')}
            <strong>{instance.instanceName}</strong>
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* QR Code Display */}
          <div className="flex flex-col items-center gap-4">
            {instance.qrcode?.base64 ? (
              <div className="rounded-lg border-2 border-sidebar-border bg-white p-4">
                <img
                  src={instance.qrcode.base64}
                  alt={t('messages.qrAlt')}
                  className="h-64 w-64"
                />
              </div>
            ) : (
              <div className="flex h-64 w-64 items-center justify-center rounded-lg border-2 border-dashed border-sidebar-border bg-sidebar">
                <div className="text-center">
                  <QrCode className="mx-auto h-12 w-12 text-sidebar-foreground/40" />
                  <p className="mt-2 text-sm text-sidebar-foreground/60">
                    {t('messages.awaitingQr')}
                  </p>
                </div>
              </div>
            )}

            {/* Pairing Code (if available) */}
            {instance.qrcode?.pairingCode && (
              <div className="w-full rounded-lg bg-sidebar-accent p-3 text-center">
                <p className="text-xs text-sidebar-foreground/60">
                  {t('messages.pairingCode')}
                </p>
                <p className="mt-1 font-mono text-lg font-semibold text-sidebar-foreground">
                  {instance.qrcode.pairingCode}
                </p>
              </div>
            )}
          </div>

          {/* Instructions */}
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

          {/* Actions */}
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={handleRefresh}
              disabled={isRefreshing}
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
            <Button
              variant="outline"
              onClick={() => onOpenChange(false)}
              className="bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
