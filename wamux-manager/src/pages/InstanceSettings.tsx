import { useEffect, useMemo, useState, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { ArrowLeft, Save, Trash2, Power, Eye, EyeOff, Copy, Check, Network, RefreshCw, Pencil, X } from "lucide-react";
import { Button } from "@/components/ui";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import * as instancesApi from "@/services/api/instances";
import type { Instance, InstanceOverview, ProxyConfig, ProxyTestResult } from "@/types/instance";
import { deviceLabel } from "@/utils/device";
import { updateInstanceInCache } from '@/hooks/useInstances';
import { useI18n } from '@/i18n/I18nContext';

const webhookSchema = z.object({
  webhookUrl: z.string().url("instanceSettings.validation.invalidUrl").optional().or(z.literal("")),
  subscribe: z.array(z.string()).optional(),
  rabbitmqEnable: z.string().optional(),
  websocketEnable: z.string().optional(),
  natsEnable: z.string().optional(),
});

const advancedSchema = z.object({
  alwaysOnline: z.boolean().optional(),
  rejectCall: z.boolean().optional(),
  readMessages: z.boolean().optional(),
  ignoreGroups: z.boolean().optional(),
  ignoreStatus: z.boolean().optional(),
});

type WebhookFormData = z.infer<typeof webhookSchema>;
type AdvancedFormData = z.infer<typeof advancedSchema>;

// Parse the events string ("MESSAGE,QRCODE,CONNECTION") into an array. Declared
// outside the component so the parse is a pure function of the string.
function parseEvents(events: string): string[] {
  return events
    .split(",")
    .map((e) => e.trim())
    .filter(Boolean);
}

const availableEvents = [
  "ALL",
  "MESSAGE",
  "READ_RECEIPT",
  "PRESENCE",
  "HISTORY_SYNC",
  "CHAT_PRESENCE",
  "CALL",
  "CONNECTION",
  "QRCODE",
  "LABEL",
  "CONTACT",
  "GROUP",
  "NEWSLETTER",
];

export default function InstanceSettings() {
  const { t } = useI18n();
  const { instanceId } = useParams<{ instanceId: string }>();
  const navigate = useNavigate();
  const [instance, setInstance] = useState<Instance | null>(null);
  const [overview, setOverview] = useState<InstanceOverview | null>(null);
  const [editedEvents, setEditedEvents] = useState<string[] | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [showToken, setShowToken] = useState(false);
  const [copied, setCopied] = useState(false);
  const [isEditingName, setIsEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState("");
  const [isRenaming, setIsRenaming] = useState(false);
  const isInitialized = useRef(false);
  const hasFetchedOnce = useRef(false);

  // Proxy configuration (admin-key routes: /instance/proxy/:id).
  const emptyProxy: ProxyConfig = { protocol: "http", host: "", port: "", username: "", password: "" };
  const [proxy, setProxy] = useState<ProxyConfig | null>(null);
  const [proxyForm, setProxyForm] = useState<ProxyConfig>(emptyProxy);
  const [proxyTest, setProxyTest] = useState<ProxyTestResult | null>(null);
  const [proxyBusy, setProxyBusy] = useState<null | "save" | "test" | "reconnect" | "delete">(null);

  const {
    register: registerWebhook,
    handleSubmit: handleSubmitWebhook,
    formState: { errors: webhookErrors },
    reset: resetWebhook,
  } = useForm<WebhookFormData>({
    resolver: zodResolver(webhookSchema),
  });

  const {
    register: registerAdvanced,
    handleSubmit: handleSubmitAdvanced,
    reset: resetAdvanced,
  } = useForm<AdvancedFormData>({
    resolver: zodResolver(advancedSchema),
  });

  // Fetch instance data on mount (only once)
  useEffect(() => {
    const loadInstance = async () => {
      if (!instanceId || hasFetchedOnce.current) return;

      hasFetchedOnce.current = true;

      try {
        setIsLoading(true);
        const instanceData = await instancesApi.fetchInstance(instanceId);
        setInstance(instanceData);

        // Per-instance overview (profile picture, device platform, counts).
        instancesApi
          .fetchInstanceOverview(instanceId)
          .then(setOverview)
          .catch(() => {});

        // Proxy config lives on its own admin route; null means "not set".
        const proxyData = await instancesApi.getProxy(instanceId).catch(() => null);
        setProxy(proxyData);
        if (proxyData) {
          setProxyForm({
            protocol: proxyData.protocol || "http",
            host: proxyData.host || "",
            port: proxyData.port || "",
            username: proxyData.username || "",
            password: proxyData.password || "",
          });
        }
      } catch (error) {
        console.error("Erro ao buscar instância:", error);
        toast.error(t('instanceSettings.toast.loadError'));
      } finally {
        setIsLoading(false);
      }
    };

    loadInstance();
  }, [instanceId, t]);

  // Populate forms when instance data is loaded (only once)
  useEffect(() => {
    if (!instance || isInitialized.current) return;

    // Populate webhook form
    resetWebhook({
      webhookUrl: instance.webhook || "",
      rabbitmqEnable: instance.rabbitmqEnable || "",
      websocketEnable: instance.websocketEnable || "",
      natsEnable: instance.natsEnable || "",
    });

    // Populate advanced settings form
    resetAdvanced({
      alwaysOnline: instance.alwaysOnline || false,
      rejectCall: instance.rejectCall || false,
      readMessages: instance.readMessages || false,
      ignoreGroups: instance.ignoreGroups || false,
      ignoreStatus: instance.ignoreStatus || false,
    });

    isInitialized.current = true;
  }, [instance, resetWebhook, resetAdvanced]);

  // Derive the event checkboxes from the instance. parseEvents returns a new
  // array each render, so it is memoized; setEditedEvents still records the
  // user's edits. This avoids setting state in an effect.
  const parsedEvents = useMemo(
    () => (instance?.events ? parseEvents(instance.events) : []),
    [instance]
  );
  const selectedEvents = editedEvents ?? parsedEvents;

  const toggleEvent = (event: string) => {
    setEditedEvents((prev) => {
      const current = prev ?? parsedEvents;
      if (event === "ALL") {
        if (current.includes("ALL")) {
          return [];
        }
        return ["ALL"];
      }

      if (current.includes("ALL")) {
        return [event];
      }

      if (current.includes(event)) {
        return current.filter((e) => e !== event);
      }
      return [...current, event];
    });
  };

  const onSubmitWebhook = async (data: WebhookFormData) => {
    if (!instance?.apikey || !instanceId) {
      toast.error(t('instanceSettings.toast.tokenNotFound'));
      return;
    }

    try {
      setIsSaving(true);
      const config = {
        webhookUrl: data.webhookUrl || "",
        subscribe: selectedEvents,
        rabbitmqEnable: data.rabbitmqEnable || "",
        websocketEnable: data.websocketEnable || "",
        natsEnable: data.natsEnable || "",
      };

      await instancesApi.connectInstance(instance.apikey, config);
      toast.success(t('instanceSettings.toast.webhookUpdated'));

      // Refetch instance data
      const updatedInstance = await instancesApi.fetchInstance(instanceId);
      setInstance(updatedInstance);
    } catch (error) {
      console.error("Erro ao atualizar webhook:", error);
      toast.error(
        error instanceof Error ? error.message : t('instanceSettings.toast.webhookUpdateError')
      );
    } finally {
      setIsSaving(false);
    }
  };

  const onSubmitAdvanced = async (data: AdvancedFormData) => {
    if (!instance?.apikey || !instance?.id || !instanceId) {
      toast.error(t('instanceSettings.toast.tokenNotFound'));
      return;
    }

    try {
      setIsSaving(true);
      await instancesApi.updateAdvancedSettings(
        instance.id,
        instance.apikey,
        data
      );
      toast.success(t('instanceSettings.toast.advancedUpdated'));

      // Refetch instance data
      const updatedInstance = await instancesApi.fetchInstance(instanceId);
      setInstance(updatedInstance);
    } catch (error) {
      console.error("Erro ao atualizar configurações:", error);
      toast.error(
        error instanceof Error
          ? error.message
          : t('instanceSettings.toast.advancedUpdateError')
      );
    } finally {
      setIsSaving(false);
    }
  };

  const handleDisconnect = async () => {
    if (!instance?.apikey || !instanceId) {
      toast.error(t('instanceSettings.toast.tokenNotFound'));
      return;
    }

    try {
      toast.info(t('instanceSettings.toast.disconnecting', { name: instance.instanceName }));
      await instancesApi.logoutInstance(instance.apikey);

      // Refetch instance data
      const updatedInstance = await instancesApi.fetchInstance(instanceId);
      setInstance(updatedInstance);

      toast.success(t('instanceSettings.toast.disconnected', { name: instance.instanceName }));
    } catch (error) {
      console.error("Erro ao desconectar instância:", error);
      toast.error(
        error instanceof Error ? error.message : t('instanceSettings.toast.disconnectError')
      );
    }
  };

  const handleDelete = async () => {
    if (!instance?.id) {
      toast.error(t('instanceSettings.toast.idNotFound'));
      return;
    }

    const confirmed = window.confirm(
      t('instanceSettings.confirm.delete', { name: instance.instanceName })
    );

    if (!confirmed) return;

    try {
      toast.info(t('instanceSettings.toast.deleting', { name: instance.instanceName }));
      await instancesApi.deleteInstance(instance.id);
      toast.success(t('instanceSettings.toast.deleted', { name: instance.instanceName }));
      navigate("/manager/instances");
    } catch (error) {
      console.error("Erro ao deletar instância:", error);
      toast.error(
        error instanceof Error ? error.message : t('instanceSettings.toast.deleteError')
      );
    }
  };

  const handleCopyToken = async () => {
    if (!instance?.apikey) return;
    try {
      await navigator.clipboard.writeText(instance.apikey);
      setCopied(true);
      toast.success(t('instanceSettings.toast.tokenCopied'));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error(t('instanceSettings.toast.copyError'));
    }
  };

  const startEditName = () => {
    if (!instance) return;
    setNameDraft(instance.instanceName);
    setIsEditingName(true);
  };

  const cancelEditName = () => {
    setIsEditingName(false);
    setNameDraft("");
  };

  const handleRename = async () => {
    if (!instanceId) return;

    const name = nameDraft.trim();
    if (!name) {
      toast.error(t('instanceSettings.toast.nameRequired'));
      return;
    }
    if (name === instance?.instanceName) {
      cancelEditName();
      return;
    }

    setIsRenaming(true);
    try {
      const updated = await instancesApi.renameInstance(instanceId, name);
      setInstance(updated);
      // Keep the instances list in sync without a full refetch. The list is
      // keyed by name, so update the entry under its previous name.
      if (instance?.instanceName) {
        updateInstanceInCache(instance.instanceName, {
          instanceName: updated.instanceName,
          profileName: updated.profileName,
        });
      }
      setIsEditingName(false);
      setNameDraft("");
      toast.success(t('instanceSettings.toast.renamed'));
    } catch (error) {
      console.error("Erro ao renomear instância:", error);
      toast.error(
        error instanceof Error ? error.message : t('instanceSettings.toast.renameError')
      );
    } finally {
      setIsRenaming(false);
    }
  };

  const proxyErrorMessage = (e: unknown, fallback: string) =>
    e instanceof Error ? e.message : fallback;

  const handleSaveProxy = async () => {
    if (!instanceId) return;
    if (!proxyForm.host.trim() || !proxyForm.port.trim()) {
      toast.error(t('instanceSettings.toast.proxyHostPortRequired'));
      return;
    }
    setProxyBusy("save");
    try {
      await instancesApi.setProxy(instanceId, proxyForm);
      const saved = await instancesApi.getProxy(instanceId);
      setProxy(saved);
      toast.success(t('instanceSettings.toast.proxySaved'));
    } catch (e) {
      toast.error(proxyErrorMessage(e, t('instanceSettings.toast.proxySaveError')));
    } finally {
      setProxyBusy(null);
    }
  };

  const handleTestProxy = async () => {
    if (!instanceId) return;
    setProxyBusy("test");
    try {
      // With a host filled in, test what is in the form; otherwise test what is
      // already saved for the instance.
      const result = await instancesApi.testProxy(
        instanceId,
        proxyForm.host.trim() ? proxyForm : undefined
      );
      setProxyTest(result);
      if (result.ok) toast.success(t('instanceSettings.toast.proxyOk'));
      else toast.error(result.error || t('instanceSettings.toast.proxyNoResponse'));
    } catch (e) {
      toast.error(proxyErrorMessage(e, t('instanceSettings.toast.proxyTestError')));
    } finally {
      setProxyBusy(null);
    }
  };

  const handleReconnectProxy = async () => {
    if (!instanceId) return;
    setProxyBusy("reconnect");
    try {
      await instancesApi.reconnectProxy(instanceId);
      toast.success(t('instanceSettings.toast.proxyReconnecting'));
    } catch (e) {
      toast.error(proxyErrorMessage(e, t('instanceSettings.toast.proxyReconnectError')));
    } finally {
      setProxyBusy(null);
    }
  };

  const handleDeleteProxy = async () => {
    if (!instanceId) return;
    if (!window.confirm(t('instanceSettings.confirm.removeProxy'))) return;
    setProxyBusy("delete");
    try {
      await instancesApi.deleteProxy(instanceId);
      setProxy(null);
      setProxyForm(emptyProxy);
      setProxyTest(null);
      toast.success(t('instanceSettings.toast.proxyRemoved'));
    } catch (e) {
      toast.error(proxyErrorMessage(e, t('instanceSettings.toast.proxyRemoveError')));
    } finally {
      setProxyBusy(null);
    }
  };

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <p className="text-muted-foreground">{t('common.loading')}</p>
        </div>
      </div>
    );
  }

  if (!instance) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-semibold text-foreground mb-2">
            {t('instanceSettings.notFoundTitle')}
          </h2>
          <p className="text-muted-foreground mb-4">
            {t('instanceSettings.notFoundDescription', { id: instanceId ?? '' })}
          </p>
          <Button onClick={() => navigate("/manager/instances")}>
            <ArrowLeft className="h-4 w-4 mr-2" />
            {t('instanceSettings.backToInstances')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="border-b border-sidebar-border bg-sidebar p-4 sm:p-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => navigate("/manager/instances")}
              title={t('instanceSettings.backToInstances')}
              aria-label={t('instanceSettings.backToInstances')}
              className="text-sidebar-foreground hover:bg-sidebar-accent"
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
            <div>
              <h1 className="text-2xl font-bold text-foreground">
                {t('instanceSettings.pageTitle')}
              </h1>
              <p className="text-sm text-muted-foreground">
                {instance.instanceName}
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4 sm:p-6">
        <div className="max-w-4xl mx-auto space-y-6">
          {/* Instance Info Card */}
          <div className="rounded-lg border border-sidebar-border bg-card p-6">
            <h2 className="text-lg font-semibold text-foreground mb-4">
              {t('instanceSettings.info.title')}
            </h2>
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="text-sm font-medium text-foreground">
                    {t('instanceSettings.field.instanceName')}
                  </label>
                  {isEditingName ? (
                    <div className="mt-1 flex items-center gap-2">
                      <input
                        type="text"
                        value={nameDraft}
                        autoFocus
                        maxLength={100}
                        onChange={(e) => setNameDraft(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") {
                            e.preventDefault();
                            handleRename();
                          } else if (e.key === "Escape") {
                            cancelEditName();
                          }
                        }}
                        className="min-w-0 flex-1 rounded-md border border-input bg-background px-3 py-1.5 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                      />
                      <Button
                        type="button"
                        size="icon"
                        onClick={handleRename}
                        disabled={isRenaming}
                        title={t('instanceSettings.action.saveName')}
                      >
                        <Check className="h-4 w-4" />
                      </Button>
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        onClick={cancelEditName}
                        disabled={isRenaming}
                        title={t('common.cancel')}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  ) : (
                    <div className="mt-1 flex items-center gap-2">
                      <p className="text-sm text-muted-foreground">
                        {instance.instanceName}
                      </p>
                      <button
                        type="button"
                        onClick={startEditName}
                        className="text-muted-foreground hover:text-foreground transition-colors"
                        title={t('instanceSettings.action.editName')}
                      >
                        <Pencil size={16} />
                      </button>
                    </div>
                  )}
                </div>
                <div>
                  <label className="text-sm font-medium text-foreground">
                    {t('instanceSettings.field.instanceToken')}
                  </label>
                  <div className="mt-1 flex min-w-0 items-center gap-2">
                    <p className="min-w-0 flex-1 break-all text-sm text-muted-foreground font-mono">
                      {showToken ? (instance.apikey || '') : '•'.repeat((instance.apikey || '').length)}
                    </p>
                    <button
                      type="button"
                      onClick={() => setShowToken(!showToken)}
                      className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                      title={showToken ? t('instanceSettings.action.hideToken') : t('instanceSettings.action.showToken')}
                    >
                      {showToken ? <EyeOff size={18} /> : <Eye size={18} />}
                    </button>
                    <button
                      type="button"
                      onClick={handleCopyToken}
                      className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                      title={t('instanceSettings.action.copyToken')}
                    >
                      {copied ? (
                        <Check size={18} className="text-green-500" />
                      ) : (
                        <Copy size={18} />
                      )}
                    </button>
                  </div>
                </div>
                <div>
                  <label className="text-sm font-medium text-foreground">
                    {t('instanceSettings.field.status')}
                  </label>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {instance.status === "open" ? t('instanceSettings.status.connected') : t('instanceSettings.status.disconnected')}
                  </p>
                </div>
                {instance.owner && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      {t('instanceSettings.field.number')}
                    </label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {instance.owner}
                    </p>
                  </div>
                )}
                {instance.profileName && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      {t('instanceSettings.field.profileName')}
                    </label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {instance.profileName}
                    </p>
                  </div>
                )}
                {deviceLabel(overview?.platform) && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      {t('instanceSettings.field.device')}
                    </label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {deviceLabel(overview?.platform)}
                    </p>
                  </div>
                )}
                {overview?.businessName && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      {t('instanceSettings.field.businessName')}
                    </label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {overview.businessName}
                    </p>
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Webhook Settings Card */}
          <form onSubmit={handleSubmitWebhook(onSubmitWebhook)}>
            <div className="rounded-lg border border-sidebar-border bg-card p-6">
              <h2 className="text-lg font-semibold text-foreground mb-4">
                {t('instanceSettings.webhook.title')}
              </h2>
              <div className="space-y-4">
                <div>
                  <label
                    htmlFor="webhookUrl"
                    className="block text-sm font-medium text-foreground mb-1"
                  >
                    {t('instanceSettings.webhook.url')}
                  </label>
                  <input
                    id="webhookUrl"
                    type="url"
                    placeholder={t('instanceSettings.webhook.urlPlaceholder')}
                    {...registerWebhook("webhookUrl")}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                  {webhookErrors.webhookUrl && (
                    <p className="mt-1 text-sm text-destructive">
                      {t(webhookErrors.webhookUrl.message ?? '')}
                    </p>
                  )}
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('instanceSettings.webhook.urlHelp')}
                  </p>
                </div>

                {/* Events Selection */}
                <div>
                  <label className="block text-sm font-medium text-foreground mb-2">
                    {t('instanceSettings.webhook.events')}
                  </label>
                  <div className="space-y-2 rounded-md border border-input p-3 max-h-60 overflow-y-auto">
                    {/* ALL Option */}
                    <div className="p-2 rounded-md bg-primary/10 border border-primary/20">
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={selectedEvents.includes("ALL")}
                          onChange={() => toggleEvent("ALL")}
                          className="rounded border-input w-4 h-4"
                        />
                        <span className="text-sm font-semibold text-primary">
                          {t('instanceSettings.events.ALL')}
                        </span>
                      </label>
                    </div>

                    {/* Individual Events */}
                    <div className="grid grid-cols-2 gap-2">
                      {availableEvents
                        .filter((e) => e !== "ALL")
                        .map((event) => (
                        <label
                          key={event}
                          className="flex items-center gap-2 text-sm cursor-pointer hover:bg-accent p-2 rounded"
                        >
                          <input
                            type="checkbox"
                              checked={
                                selectedEvents.includes(event) ||
                                selectedEvents.includes("ALL")
                              }
                            onChange={() => toggleEvent(event)}
                              disabled={selectedEvents.includes("ALL")}
                            className="rounded border-input"
                          />
                            <span
                              className={
                                selectedEvents.includes("ALL")
                                  ? "text-muted-foreground"
                                  : "text-foreground"
                              }
                            >
                            {t(`instanceSettings.events.${event}`)}
                          </span>
                        </label>
                      ))}
                    </div>
                  </div>
                </div>

                {/* Event Producers */}
                <div className="grid grid-cols-3 gap-4">
                  <div>
                    <label
                      htmlFor="rabbitmqEnable"
                      className="block text-sm font-medium text-foreground mb-1"
                    >
                      RabbitMQ
                    </label>
                    <select
                      id="rabbitmqEnable"
                      {...registerWebhook("rabbitmqEnable")}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                    >
                      <option value="">{t('instanceSettings.option.default')}</option>
                      <option value="enabled">{t('instanceSettings.option.enabled')}</option>
                      <option value="disabled">{t('instanceSettings.option.disabled')}</option>
                    </select>
                  </div>

                  <div>
                    <label
                      htmlFor="websocketEnable"
                      className="block text-sm font-medium text-foreground mb-1"
                    >
                      WebSocket
                    </label>
                    <select
                      id="websocketEnable"
                      {...registerWebhook("websocketEnable")}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                    >
                      <option value="">{t('instanceSettings.option.default')}</option>
                      <option value="enabled">{t('instanceSettings.option.enabled')}</option>
                      <option value="disabled">{t('instanceSettings.option.disabled')}</option>
                    </select>
                  </div>

                  <div>
                    <label
                      htmlFor="natsEnable"
                      className="block text-sm font-medium text-foreground mb-1"
                    >
                      NATS
                    </label>
                    <select
                      id="natsEnable"
                      {...registerWebhook("natsEnable")}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                    >
                      <option value="">{t('instanceSettings.option.default')}</option>
                      <option value="enabled">{t('instanceSettings.option.enabled')}</option>
                      <option value="disabled">{t('instanceSettings.option.disabled')}</option>
                    </select>
                  </div>
                </div>

                <div className="flex justify-end">
                  <Button type="submit" disabled={isSaving} className="gap-2">
                    <Save className="h-4 w-4" />
                    {isSaving ? t('instanceSettings.action.saving') : t('instanceSettings.webhook.save')}
                  </Button>
                </div>
              </div>
            </div>
          </form>

          {/* Proxy Settings Card */}
          <div className="rounded-lg border border-sidebar-border bg-card p-6">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="flex items-center gap-2 text-lg font-semibold text-foreground">
                <Network className="h-5 w-5" />
                {t('instanceSettings.proxy.title')}
              </h2>
              <span
                className={`rounded-full border px-2 py-0.5 text-xs ${
                  proxy
                    ? "border-green-500/40 bg-green-500/10 text-green-500"
                    : "border-sidebar-border text-muted-foreground"
                }`}
              >
                {proxy ? t('instanceSettings.proxy.configured') : t('instanceSettings.proxy.notConfigured')}
              </span>
            </div>

            <div className="space-y-4">
              <p className="text-xs text-muted-foreground">
                {t('instanceSettings.proxy.description')}
              </p>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    {t('instanceSettings.proxy.protocol')}
                  </label>
                  <select
                    value={proxyForm.protocol || "http"}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, protocol: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  >
                    <option value="http">http</option>
                    <option value="https">https</option>
                    <option value="socks5">socks5</option>
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    {t('instanceSettings.proxy.host')}
                  </label>
                  <input
                    type="text"
                    placeholder={t('instanceSettings.proxy.hostPlaceholder')}
                    value={proxyForm.host}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, host: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    {t('instanceSettings.proxy.port')}
                  </label>
                  <input
                    type="text"
                    placeholder={t('instanceSettings.proxy.portPlaceholder')}
                    value={proxyForm.port}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, port: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    {t('instanceSettings.proxy.username')} <span className="text-muted-foreground">{t('instanceSettings.proxy.optional')}</span>
                  </label>
                  <input
                    type="text"
                    value={proxyForm.username}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, username: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    {t('instanceSettings.proxy.password')} <span className="text-muted-foreground">{t('instanceSettings.proxy.optional')}</span>
                  </label>
                  <input
                    type="password"
                    placeholder={proxy?.hasPassword ? t('instanceSettings.proxy.passwordUnchanged') : t('instanceSettings.proxy.passwordOptionalPlaceholder')}
                    value={proxyForm.password}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, password: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('instanceSettings.proxy.passwordHelp')}
                  </p>
                </div>
              </div>

              {proxyTest && (
                <div
                  className={`rounded-md border p-3 text-xs ${
                    proxyTest.ok
                      ? "border-green-500/40 bg-green-500/10 text-green-400"
                      : "border-destructive/40 bg-destructive/10 text-destructive"
                  }`}
                >
                  {proxyTest.ok ? (
                    <div className="space-y-1">
                      <p className="font-medium">
                        {proxyTest.protocol
                          ? t('instanceSettings.proxy.testOkProtocol', { protocol: proxyTest.protocol })
                          : t('instanceSettings.proxy.testOk')}
                      </p>
                      <p>
                        {t('instanceSettings.proxy.exitIp')}{" "}
                        <span className="font-mono">{proxyTest.ip}</span>{" "}
                        {proxyTest.anonymous ? t('instanceSettings.proxy.anonymous') : t('instanceSettings.proxy.notAnonymous')}
                      </p>
                      <p>
                        {t('instanceSettings.proxy.whatsappReachable')}{" "}
                        {proxyTest.whatsappReachable ? t('instanceSettings.yes') : t('instanceSettings.no')}
                      </p>
                      {proxyTest.latencyMs !== undefined && (
                        <p>{t('instanceSettings.proxy.latency', { ms: proxyTest.latencyMs })}</p>
                      )}
                    </div>
                  ) : (
                    <div className="space-y-1">
                      <p className="font-medium">{t('instanceSettings.proxy.testFailed')}</p>
                      {proxyTest.error && <p>{proxyTest.error}</p>}
                    </div>
                  )}
                </div>
              )}

              <div className="flex flex-wrap justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleDeleteProxy}
                  disabled={!proxy || proxyBusy !== null}
                  className="gap-2"
                >
                  <Trash2 className="h-4 w-4" />
                  {t('instanceSettings.proxy.remove')}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleReconnectProxy}
                  disabled={!proxy || proxyBusy !== null}
                  className="gap-2"
                >
                  <RefreshCw className="h-4 w-4" />
                  {t('instanceSettings.proxy.reconnect')}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleTestProxy}
                  disabled={proxyBusy !== null}
                  className="gap-2"
                >
                  <Network className="h-4 w-4" />
                  {proxyBusy === "test" ? t('instanceSettings.proxy.testing') : t('instanceSettings.proxy.test')}
                </Button>
                <Button
                  type="button"
                  onClick={handleSaveProxy}
                  disabled={proxyBusy !== null}
                  className="gap-2"
                >
                  <Save className="h-4 w-4" />
                  {proxyBusy === "save" ? t('instanceSettings.action.saving') : t('instanceSettings.proxy.save')}
                </Button>
              </div>
            </div>
          </div>

          {/* Advanced Settings Card */}
          <form onSubmit={handleSubmitAdvanced(onSubmitAdvanced)}>
            <div className="rounded-lg border border-sidebar-border bg-card p-6">
              <h2 className="text-lg font-semibold text-foreground mb-4">
                {t('instanceSettings.advanced.title')}
              </h2>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <label
                      htmlFor="alwaysOnline"
                      className="text-sm font-medium text-foreground cursor-pointer"
                    >
                      {t('instanceSettings.advanced.alwaysOnline')}
                    </label>
                    <p className="text-xs text-muted-foreground">
                      {t('instanceSettings.advanced.alwaysOnlineHelp')}
                    </p>
                  </div>
                  <input
                    id="alwaysOnline"
                    type="checkbox"
                    {...registerAdvanced("alwaysOnline")}
                    className="rounded border-input w-4 h-4"
                  />
                </div>

                <div className="flex items-center justify-between">
                  <div>
                    <label
                      htmlFor="rejectCall"
                      className="text-sm font-medium text-foreground cursor-pointer"
                    >
                      {t('instanceSettings.advanced.rejectCall')}
                    </label>
                    <p className="text-xs text-muted-foreground">
                      {t('instanceSettings.advanced.rejectCallHelp')}
                    </p>
                  </div>
                  <input
                    id="rejectCall"
                    type="checkbox"
                    {...registerAdvanced("rejectCall")}
                    className="rounded border-input w-4 h-4"
                  />
                </div>

                <div className="flex items-center justify-between">
                  <div>
                    <label
                      htmlFor="readMessages"
                      className="text-sm font-medium text-foreground cursor-pointer"
                    >
                      {t('instanceSettings.advanced.readMessages')}
                    </label>
                    <p className="text-xs text-muted-foreground">
                      {t('instanceSettings.advanced.readMessagesHelp')}
                    </p>
                  </div>
                  <input
                    id="readMessages"
                    type="checkbox"
                    {...registerAdvanced("readMessages")}
                    className="rounded border-input w-4 h-4"
                  />
                </div>

                <div className="flex items-center justify-between">
                  <div>
                    <label
                      htmlFor="ignoreGroups"
                      className="text-sm font-medium text-foreground cursor-pointer"
                    >
                      {t('instanceSettings.advanced.ignoreGroups')}
                    </label>
                    <p className="text-xs text-muted-foreground">
                      {t('instanceSettings.advanced.ignoreGroupsHelp')}
                    </p>
                  </div>
                  <input
                    id="ignoreGroups"
                    type="checkbox"
                    {...registerAdvanced("ignoreGroups")}
                    className="rounded border-input w-4 h-4"
                  />
                </div>

                <div className="flex items-center justify-between">
                  <div>
                    <label
                      htmlFor="ignoreStatus"
                      className="text-sm font-medium text-foreground cursor-pointer"
                    >
                      {t('instanceSettings.advanced.ignoreStatus')}
                    </label>
                    <p className="text-xs text-muted-foreground">
                      {t('instanceSettings.advanced.ignoreStatusHelp')}
                    </p>
                  </div>
                  <input
                    id="ignoreStatus"
                    type="checkbox"
                    {...registerAdvanced("ignoreStatus")}
                    className="rounded border-input w-4 h-4"
                  />
                </div>

                <div className="flex justify-end">
                  <Button type="submit" disabled={isSaving} className="gap-2">
                    <Save className="h-4 w-4" />
                    {isSaving ? t('instanceSettings.action.saving') : t('instanceSettings.advanced.save')}
                  </Button>
                </div>
              </div>
            </div>
          </form>

          {/* Danger Zone Card */}
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-6">
            <h2 className="text-lg font-semibold text-destructive mb-4">
              {t('instanceSettings.danger.title')}
            </h2>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-medium text-foreground">
                    {t('instanceSettings.danger.disconnectTitle')}
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    {t('instanceSettings.danger.disconnectHelp')}
                  </p>
                </div>
                <Button
                  variant="destructive"
                  onClick={handleDisconnect}
                  className="gap-2"
                >
                  <Power className="h-4 w-4" />
                  {t('instanceSettings.danger.disconnect')}
                </Button>
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-medium text-foreground">
                    {t('instanceSettings.danger.deleteTitle')}
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    {t('instanceSettings.danger.deleteHelp')}
                  </p>
                </div>
                <Button
                  variant="destructive"
                  onClick={handleDelete}
                  className="gap-2"
                >
                  <Trash2 className="h-4 w-4" />
                  {t('instanceSettings.danger.delete')}
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
