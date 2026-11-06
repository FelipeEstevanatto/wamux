import { useEffect, useMemo, useState, useCallback, useRef } from 'react';
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
} from '@/components/ui';
import { InstanceCardsSkeleton } from '@/components/base/PageSkeleton';
import { Trash2, Layers, PowerOff } from 'lucide-react';
import { toast } from 'sonner';

import useInstances from '@/hooks/useInstances';
import { InstanceCard, InstancesHeader, CreateInstanceModal, QRCodeModal } from '@/components/instances';
import SendMessageModal from '@/components/instances/SendMessageModal';
import EmptyState from '@/components/base/EmptyState';
import type { Instance } from '@/types/instance';
import * as instancesApi from '@/services/api/instances';
import { useNavigate } from 'react-router-dom';
import { useI18n } from '@/i18n/I18nContext';

const wait = (ms: number) =>
  new Promise((resolve) => setTimeout(resolve, ms));

/**
 * The backend may need a couple of seconds to emit the first QR event. Poll
 * briefly instead of sleeping unconditionally: most connections show a code on
 * the first try, and slow ones still succeed without the old fixed 2s delay.
 */
async function fetchQrCodeWithRetry(instanceToken: string, attempts = 4) {
  let lastError: unknown;

  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      const data = await instancesApi.getQrCode(instanceToken);
      if (data.qrcode || data.paired) return data;
      lastError = new Error('QR code not ready');
    } catch (error) {
      lastError = error;
    }

    if (attempt < attempts - 1) await wait(400);
  }

  throw lastError instanceof Error
    ? lastError
    : new Error('QR code not available');
}

export default function Instances() {
  const navigate = useNavigate();
  const { t } = useI18n();
  const { instances, isLoading, fetchInstances, refreshInstances, isRefreshing, removeInstance, updateInstance, overviews } =
    useInstances();
  const [query, setQuery] = useState('');
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [qrcodeModal, setQrcodeModal] = useState<{
    isOpen: boolean;
    instance: Instance | null;
    isLoading: boolean;
    isPaired: boolean;
    error: string | null;
  }>({
    isOpen: false,
    instance: null,
    isLoading: false,
    isPaired: false,
    error: null,
  });
  const [connectingInstance, setConnectingInstance] = useState<string | null>(
    null
  );
  // Incremented when the user cancels/switches flows so late async results do
  // not reopen a dialog the user already closed.
  const connectAttemptRef = useRef(0);
  // Synchronous guard against a double click landing before the state update.
  const connectingInstanceRef = useRef<string | null>(null);

  // Pagination states
  const [currentPage] = useState(1);
  const [perPage] = useState(24);
  const [isDeleting, setIsDeleting] = useState<string | null>(null);
  const [deleteModal, setDeleteModal] = useState<{
    isOpen: boolean;
    instance: Instance | null;
    confirmationText: string;
  }>({
    isOpen: false,
    instance: null,
    confirmationText: '',
  });
  const [disconnectModal, setDisconnectModal] = useState<{
    isOpen: boolean;
    instance: Instance | null;
  }>({
    isOpen: false,
    instance: null,
  });
  const [isDisconnecting, setIsDisconnecting] = useState<string | null>(null);
  const [sendMessageModal, setSendMessageModal] = useState<{
    isOpen: boolean;
    instance: Instance | null;
  }>({
    isOpen: false,
    instance: null,
  });

  // Ref to track if initial fetch was done
  const initialFetchDone = useRef(false);

  useEffect(() => {
    // Only fetch once on mount
    if (!initialFetchDone.current) {
      fetchInstances();
      initialFetchDone.current = true;
    }

    // Polling is handled centrally by the useInstances query (refetchInterval),
    // so the page no longer needs its own interval.
  }, [fetchInstances]);

  const { paginatedInstances, totalCount } = useMemo(() => {
    // First filter by search query
    const filtered = query
      ? instances.filter((i: Instance) => {
          const q = query.toLowerCase();
          return (
            i.instanceName?.toLowerCase().includes(q) ||
            i.profileName?.toLowerCase().includes(q) ||
            i.owner?.toLowerCase().includes(q)
          );
        })
      : instances;

    // Then paginate
    const startIndex = (currentPage - 1) * perPage;
    const endIndex = startIndex + perPage;
    const paginated = filtered.slice(startIndex, endIndex);

    return {
      paginatedInstances: paginated,
      totalCount: filtered.length,
    };
  }, [instances, query, currentPage, perPage]);

  const handleNewInstance = useCallback(() => {
    setCreateModalOpen(true);
  }, []);

  const handleSettings = useCallback(
    (instance: Instance) => {
      navigate(`/manager/instances/${instance.id}/settings`);
    },
    [navigate]
  );

  const handleConnect = useCallback(
    async (instance: Instance) => {
      if (!instance.apikey) {
        toast.error(t('instances.tokenNotFound'));
        return;
      }
      if (connectingInstanceRef.current === instance.instanceName) return;

      const attempt = ++connectAttemptRef.current;
      const isCurrent = () => attempt === connectAttemptRef.current;

      connectingInstanceRef.current = instance.instanceName;
      setConnectingInstance(instance.instanceName);

      // Open the QR dialog right away: the modal itself is the progress UI.
      setQrcodeModal({
        isOpen: true,
        instance: { ...instance, qrcode: undefined },
        isLoading: true,
        isPaired: false,
        error: null,
      });

      try {
        // Persistent connection settings (webhook, events, producers and
        // advanced options) live in Instance Settings. An empty body keeps
        // them untouched and only starts the session.
        await instancesApi.connectInstance(instance.apikey, {});

        // Retry briefly instead of sleeping for a fixed two seconds on every
        // connection; most instances have a QR code on the first try.
        const qrData = await fetchQrCodeWithRetry(instance.apikey);
        if (!isCurrent()) return;

        setQrcodeModal((prev) =>
          prev.instance?.id === instance.id
            ? {
                ...prev,
                instance: {
                  ...prev.instance,
                  qrcode: qrData.qrcode
                    ? {
                        base64: qrData.qrcode,
                        code: qrData.code,
                      }
                    : undefined,
                },
                isLoading: false,
                isPaired: Boolean(qrData.paired),
                error: null,
              }
            : prev
        );
      } catch (error) {
        console.error('Erro ao conectar instância:', error);

        if (!isCurrent()) return;

        const message =
          error instanceof Error
            ? error.message
            : t('instances.connectError');

        // Keep the dialog open with an inline error/retry state instead of
        // closing it and dropping the user back to the list.
        setQrcodeModal((prev) =>
          prev.isOpen && prev.instance?.id === instance.id
            ? { ...prev, isLoading: false, error: message }
            : prev
        );
        toast.error(message);
      } finally {
        connectingInstanceRef.current = null;
        setConnectingInstance((prev) =>
          prev === instance.instanceName ? null : prev
        );
      }
    },
    [t]
  );

  // The card's button only opens the confirmation; the actual logout runs from
  // the modal, so disconnecting is never a single stray click.
  const openDisconnectModal = useCallback((instance: Instance) => {
    setDisconnectModal({ isOpen: true, instance });
  }, []);

  const closeDisconnectModal = () => {
    setDisconnectModal({ isOpen: false, instance: null });
  };

  const handleConfirmDisconnect = useCallback(async () => {
    const instance = disconnectModal.instance;
    if (!instance) return;

    if (!instance.apikey) {
      toast.error(t('instances.tokenNotFound'));
      return;
    }

    setIsDisconnecting(instance.instanceName);
    // Optimistic: show the instance as disconnected right away; the refetch
    // below (and on error) reconciles with the server.
    updateInstance(instance.instanceName, { connected: false, status: 'close' });
    try {
      toast.info(t('instances.disconnecting', { name: instance.instanceName }));
      await instancesApi.logoutInstance(instance.apikey);
      await fetchInstances();
      toast.success(
        t('instances.disconnectedToast', { name: instance.instanceName })
      );
      closeDisconnectModal();
    } catch (error) {
      console.error('Erro ao desconectar instância:', error);
      toast.error(
        error instanceof Error
          ? error.message
          : t('instances.disconnectError')
      );
      await fetchInstances();
    } finally {
      setIsDisconnecting(null);
    }
  }, [disconnectModal.instance, fetchInstances, updateInstance, t]);

  const openDeleteModal = (instance: Instance) => {
    setDeleteModal({
      isOpen: true,
      instance,
      confirmationText: '',
    });
  };

  const closeDeleteModal = () => {
    setDeleteModal({
      isOpen: false,
      instance: null,
      confirmationText: '',
    });
  };

  const closeQrcodeModal = () => {
    // Cancel any in-flight connect step: a late response must not reopen the
    // dialog the user just closed.
    connectAttemptRef.current += 1;
    setConnectingInstance(null);
    setQrcodeModal({
      isOpen: false,
      instance: null,
      isLoading: false,
      isPaired: false,
      error: null,
    });
  };

  const openSendMessageModal = (instance: Instance) => {
    setSendMessageModal({
      isOpen: true,
      instance,
    });
  };

  const closeSendMessageModal = () => {
    setSendMessageModal({
      isOpen: false,
      instance: null,
    });
  };

  /**
   * Cheap status-only poll used while the QR dialog is open. It reads a single
   * instance (instead of the whole list) so the success screen can appear
   * within ~2s of the scan, and patches the list cache so the card behind the
   * dialog updates at the same time.
   */
  const handleCheckQrcodeConnection = useCallback(async () => {
    const target = qrcodeModal.instance;
    if (!target?.id) return;

    const fresh = await instancesApi.fetchInstance(target.id);
    if (!fresh.connected) return;

    updateInstance(fresh.instanceName, fresh);
    setQrcodeModal((prev) =>
      prev.isOpen && prev.instance?.id === fresh.id
        ? {
            ...prev,
            instance: { ...prev.instance, ...fresh, qrcode: prev.instance.qrcode },
            isLoading: false,
            isPaired: false,
            error: null,
          }
        : prev
    );
  }, [qrcodeModal.instance, updateInstance]);

  const handleRefreshQrcode = useCallback(async () => {
    const target = qrcodeModal.instance;
    if (!target?.id || !target.apikey) return;

    const fresh = await instancesApi.fetchInstance(target.id);

    // If connected, just update modal and it will show success screen
    if (fresh.connected) {
      updateInstance(fresh.instanceName, fresh);
      setQrcodeModal((prev) =>
        prev.isOpen && prev.instance?.id === fresh.id
          ? {
              ...prev,
              instance: { ...prev.instance, ...fresh, qrcode: prev.instance.qrcode },
              isLoading: false,
              isPaired: false,
              error: null,
            }
          : prev
      );
      return;
    }

    // If not connected, get a fresh QR Code
    const qrData = await instancesApi.getQrCode(target.apikey);

    // Paired instances have no QR to show; the session is being resumed and
    // the dialog will flip to the success screen when it connects.
    if (qrData.paired) {
      setQrcodeModal((prev) =>
        prev.isOpen && prev.instance?.id === target.id
          ? {
              ...prev,
              instance: {
                ...prev.instance,
                ...fresh,
                qrcode: undefined,
              },
              isLoading: false,
              isPaired: true,
              error: null,
            }
          : prev
      );
      return;
    }

    if (!qrData.qrcode) throw new Error(t('instances.qrNotAvailable'));

    setQrcodeModal((prev) =>
      prev.isOpen && prev.instance?.id === target.id
        ? {
            ...prev,
            instance: {
              ...prev.instance,
              ...fresh,
              qrcode: {
                base64: qrData.qrcode,
                code: qrData.code,
                pairingCode: prev.instance.qrcode?.pairingCode,
              },
            },
            isLoading: false,
            isPaired: false,
            error: null,
          }
        : prev
    );
  }, [qrcodeModal.instance, updateInstance, t]);

  const handleConfirmDelete = async () => {
    if (!deleteModal.instance) return;

    const instanceName = deleteModal.instance.instanceName;
    const instanceId = deleteModal.instance.id;
    setIsDeleting(instanceName);
    // Optimistic: drop the card immediately; a refetch restores it on failure.
    removeInstance(instanceName);

    try {
      await instancesApi.deleteInstance(instanceId);
      toast.success(t('instances.removed', { name: instanceName }));
      closeDeleteModal();
    } catch (e: unknown) {
      console.error('Erro ao remover instância:', e);
      toast.error(
        (e as Error)?.message || t('instances.removeError')
      );

      // Refresh list on error to restore correct state
      await fetchInstances();
    } finally {
      setIsDeleting(null);
    }
  };

  const isDeleteConfirmationValid =
    deleteModal.confirmationText === deleteModal.instance?.instanceName;

  // Keep the open QR dialog in sync with the live instance list. The dialog
  // stores its own snapshot (it also carries the generated QR/pairing code),
  // but connection status comes from the shared query cache, so any poll or
  // manual refresh immediately flips the dialog to the success screen.
  const qrcodeLiveInstance = useMemo(() => {
    const snapshot = qrcodeModal.instance;
    if (!snapshot) return null;

    const live = instances.find((i) => i.id === snapshot.id);
    if (!live) return snapshot;

    return {
      ...snapshot,
      connected: live.connected,
      status: live.status,
      owner: live.owner || snapshot.owner,
      profileName: live.profileName || snapshot.profileName,
      qrcode: snapshot.qrcode,
    };
  }, [qrcodeModal.instance, instances]);

  const qrcodeOverview = qrcodeLiveInstance
    ? overviews[qrcodeLiveInstance.id]
    : undefined;

  return (
    <div className="h-full flex flex-col p-4">
      <InstancesHeader
        totalCount={totalCount}
        selectedCount={0}
        searchValue={query}
        onSearchChange={setQuery}
        onNewInstance={handleNewInstance}
        onRefresh={refreshInstances}
        isRefreshing={isRefreshing}
        onClearSelection={() => {}}
      />

      {/* View Mode Toggle (removed for now, only cards view) */}

      <div className="flex-1 overflow-auto">
        {isLoading ? (
          <InstanceCardsSkeleton />
        ) : totalCount === 0 ? (
          <EmptyState
            icon={Layers}
            title={t('instances.emptyTitle')}
            description={t('instances.emptyDescription')}
            action={{ label: t('instances.newInstance'), onClick: handleNewInstance }}
            className="h-full"
          />
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
            {paginatedInstances.map((instance: Instance) => (
              <InstanceCard
                key={instance.instanceName}
                instance={instance}
                overview={overviews[instance.id]}
                isDeleting={isDeleting}
                isConnecting={connectingInstance === instance.instanceName}
                onSettings={handleSettings}
                onDelete={openDeleteModal}
                onConnect={handleConnect}
                onDisconnect={openDisconnectModal}
                onSendMessage={openSendMessageModal}
              />
            ))}
          </div>
        )}
      </div>

      {/* Create Instance Modal */}
      <CreateInstanceModal
        open={createModalOpen}
        onOpenChange={setCreateModalOpen}
      />

      {/* QR Code Modal */}
      <QRCodeModal
        instance={qrcodeLiveInstance}
        open={qrcodeModal.isOpen}
        onOpenChange={closeQrcodeModal}
        onRefresh={handleRefreshQrcode}
        onCheckConnection={handleCheckQrcodeConnection}
        isLoading={qrcodeModal.isLoading}
        isPaired={qrcodeModal.isPaired}
        error={qrcodeModal.error}
        overview={qrcodeOverview}
      />

      {/* Send Message Modal */}
      <SendMessageModal
        instance={sendMessageModal.instance}
        open={sendMessageModal.isOpen}
        onClose={closeSendMessageModal}
      />

      {/* Disconnect Confirmation Modal */}
      <Dialog
        open={disconnectModal.isOpen}
        onOpenChange={(open) => {
          if (!open) closeDisconnectModal();
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-yellow-500 dark:text-yellow-400">
              <PowerOff className="h-5 w-5" />
              {t('instances.disconnectDialogTitle')}
            </DialogTitle>
            <DialogDescription className="text-sidebar-foreground/70 dark:text-gray-400">
              {t('instances.disconnectDialogPrefix')}{' '}
              <strong>{disconnectModal.instance?.instanceName}</strong>
              {t('instances.disconnectDialogSuffix')}
            </DialogDescription>
          </DialogHeader>

          <DialogFooter className="flex gap-2">
            <Button
              variant="outline"
              onClick={closeDisconnectModal}
              className="bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent dark:text-gray-400 dark:hover:bg-sidebar-accent"
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleConfirmDisconnect}
              disabled={isDisconnecting === disconnectModal.instance?.instanceName}
              className="bg-yellow-600 hover:bg-yellow-700 text-white dark:bg-yellow-600 dark:hover:bg-yellow-700"
            >
              {isDisconnecting === disconnectModal.instance?.instanceName
                ? t('instances.disconnectingButton')
                : t('instances.disconnect')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Modal */}
      <Dialog open={deleteModal.isOpen} onOpenChange={closeDeleteModal}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-red-400 dark:text-red-500">
              <Trash2 className="h-5 w-5" />
              {t('instances.deleteDialogTitle')}
            </DialogTitle>
            <DialogDescription className="text-sidebar-foreground/70 dark:text-gray-400">
              {t('instances.deleteDialogPrefix')}{' '}
              <strong>{deleteModal.instance?.instanceName}</strong>
              {t('instances.deleteDialogSuffix')}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div>
              <label className="text-sm font-medium text-sidebar-foreground dark:text-gray-400">
                {t('instances.deleteConfirmLabel')}
              </label>
              <Input
                placeholder={deleteModal.instance?.instanceName || ''}
                value={deleteModal.confirmationText}
                onChange={(e) =>
                  setDeleteModal((prev) => ({
                    ...prev,
                    confirmationText: e.target.value,
                  }))
                }
                className="mt-2 bg-sidebar border-sidebar-border text-sidebar-foreground placeholder:text-sidebar-foreground/50 dark:text-gray-400 dark:placeholder:text-gray-400"
              />
            </div>
          </div>

          <DialogFooter className="flex gap-2">
            <Button
              variant="outline"
              onClick={closeDeleteModal}
              className="bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent dark:text-gray-400 dark:hover:bg-sidebar-accent"
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleConfirmDelete}
              disabled={
                !isDeleteConfirmationValid ||
                isDeleting === deleteModal.instance?.instanceName
              }
              className="bg-red-600 hover:bg-red-700 text-white dark:bg-red-600 dark:hover:bg-red-700"
            >
              {isDeleting === deleteModal.instance?.instanceName
                ? t('instances.removingButton')
                : t('instances.deleteDialogTitle')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
