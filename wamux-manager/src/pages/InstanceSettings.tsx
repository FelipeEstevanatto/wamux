import { useEffect, useMemo, useState, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { ArrowLeft, Save, Trash2, Power, Eye, EyeOff, Copy, Check, Network, RefreshCw, Pencil, X, KeyRound } from "lucide-react";
import { Button, ConfirmDialog } from "@/components/ui";
import { InstanceSettingsSkeleton } from "@/components/base/PageSkeleton";
import { CloudSettingsCard } from "@/components/instances";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import * as instancesApi from "@/services/api/instances";
import type { Instance, InstanceOverview, ProxyConfig, ProxyTestResult, S3Config, HmacStatus, InstanceHealth } from "@/types/instance";
import { deviceLabel } from "@/utils/device";
import { cn } from "@/utils/cn";
import { proxyFormDirty, sameMembers } from "@/utils/settingsDirty";
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

const EMPTY_S3_CONFIG: S3Config = {
  enabled: false,
  endpoint: "",
  region: "",
  bucket: "",
  accessKey: "",
  secretKey: "",
  pathStyle: false,
  publicUrl: "",
  mediaDelivery: "base64",
};

// Parse the events string ("MESSAGE,QRCODE,CONNECTION") into an array. Declared
// outside the component so the parse is a pure function of the string.
function parseEvents(events: string): string[] {
  return events
    .split(",")
    .map((e) => e.trim())
    .filter(Boolean);
}

const SECTIONS = [
  { id: "connection", labelKey: "instanceSettings.nav.connection" },
  { id: "webhooks", labelKey: "instanceSettings.nav.webhooks" },
  { id: "events", labelKey: "instanceSettings.nav.events" },
  { id: "proxy", labelKey: "instanceSettings.nav.proxy" },
  { id: "storage", labelKey: "instanceSettings.nav.storage" },
  { id: "security", labelKey: "instanceSettings.nav.security" },
  { id: "behavior", labelKey: "instanceSettings.nav.behavior" },
  { id: "danger", labelKey: "instanceSettings.nav.danger" },
] as const;

function sectionClass(dirty: boolean) {
  return cn(
    "scroll-mt-16 rounded-lg border bg-card p-6",
    dirty ? "border-amber-500/60" : "border-sidebar-border"
  );
}

function UnsavedMark({ label }: { label: string }) {
  return (
    <span className="rounded-full border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300">
      {label}
    </span>
  );
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
  "TEMPLATE",
  "CLOUD_ACCOUNT",
  "IDENTITY",
];

export default function InstanceSettings() {
  const { t } = useI18n();
  const { instanceId } = useParams<{ instanceId: string }>();
  const navigate = useNavigate();
  const [instance, setInstance] = useState<Instance | null>(null);
  const [overview, setOverview] = useState<InstanceOverview | null>(null);
  const [editedEvents, setEditedEvents] = useState<string[] | null>(null);
  const [savingSection, setSavingSection] = useState<null | "webhook" | "behavior">(null);
  const [activeSection, setActiveSection] = useState<string>(SECTIONS[0].id);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [showToken, setShowToken] = useState(false);
  const [copied, setCopied] = useState(false);
  const [isEditingName, setIsEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState("");
  const [isRenaming, setIsRenaming] = useState(false);
  const [pendingConfirm, setPendingConfirm] = useState<null | "delete" | "disconnect" | "removeProxy">(null);
  const isInitialized = useRef(false);
  const hasFetchedOnce = useRef(false);

  // Proxy configuration (admin-key routes: /instance/proxy/:id).
  const emptyProxy: ProxyConfig = { protocol: "http", host: "", port: "", username: "", password: "" };
  const [proxy, setProxy] = useState<ProxyConfig | null>(null);
  const [proxyForm, setProxyForm] = useState<ProxyConfig>(emptyProxy);
  const [proxyTest, setProxyTest] = useState<ProxyTestResult | null>(null);
  const [proxyBusy, setProxyBusy] = useState<null | "save" | "test" | "reconnect" | "delete">(null);

  // Per-instance S3 and webhook HMAC (instance-token routes).
  const [s3Form, setS3Form] = useState<S3Config>(EMPTY_S3_CONFIG);
  const [s3Busy, setS3Busy] = useState<null | "save" | "test" | "delete">(null);
  const [hmac, setHmac] = useState<HmacStatus | null>(null);
  const [hmacKey, setHmacKey] = useState("");
  const [hmacGenerated, setHmacGenerated] = useState<string | null>(null);
  const [hmacBusy, setHmacBusy] = useState<null | "save" | "generate" | "delete">(null);
  const [health, setHealth] = useState<InstanceHealth | null>(null);

  // Phone pairing is a one-off connection action, not a persisted setting.
  const [pairPhone, setPairPhone] = useState("");
  const [pairingCode, setPairingCode] = useState<string | null>(null);
  const [isPairing, setIsPairing] = useState(false);
  const [copiedPairing, setCopiedPairing] = useState(false);

  const {
    register: registerWebhook,
    handleSubmit: handleSubmitWebhook,
    formState: { errors: webhookErrors, isDirty: webhookFormDirty },
    reset: resetWebhook,
  } = useForm<WebhookFormData>({
    resolver: zodResolver(webhookSchema),
  });

  const {
    register: registerAdvanced,
    handleSubmit: handleSubmitAdvanced,
    formState: { isDirty: behaviorDirty },
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

        // Instance-token routes. Failures are non-fatal: the section still works.
        instancesApi
          .getS3Config(instanceData.apikey || "")
          .then((config) => setS3Form({ ...EMPTY_S3_CONFIG, ...config, secretKey: "" }))
          .catch(() => {});
        instancesApi
          .getHmacStatus(instanceData.apikey || "")
          .then(setHmac)
          .catch(() => {});
        instancesApi
          .getInstanceHealth(instanceData.apikey || "")
          .then((h) => setHealth(h || null))
          .catch(() => {});
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

  // While a pairing code is on screen, poll the instance so the page flips to
  // "connected" soon after the user finishes on WhatsApp.
  useEffect(() => {
    if (!pairingCode || !instanceId || instance?.connected) return;

    const interval = window.setInterval(async () => {
      try {
        const fresh = await instancesApi.fetchInstance(instanceId);
        if (!fresh.connected) return;

        setInstance(fresh);
        updateInstanceInCache(fresh.instanceName, fresh);
        setPairingCode(null);
        setPairPhone('');
        toast.success(
          t('instanceSettings.connection.success', {
            name: fresh.instanceName,
          })
        );
      } catch {
        // Transient poll failure: keep the code and try again next tick.
      }
    }, 2500);

    return () => window.clearInterval(interval);
  }, [pairingCode, instanceId, instance?.connected, t]);

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
      setSavingSection("webhook");
      const config = {
        webhookUrl: data.webhookUrl || "",
        subscribe: selectedEvents,
        rabbitmqEnable: data.rabbitmqEnable || "",
        websocketEnable: data.websocketEnable || "",
        natsEnable: data.natsEnable || "",
      };

      await instancesApi.connectInstance(instance.apikey, config);
      resetWebhook({
        webhookUrl: data.webhookUrl || "",
        rabbitmqEnable: data.rabbitmqEnable || "",
        websocketEnable: data.websocketEnable || "",
        natsEnable: data.natsEnable || "",
      });
      setEditedEvents(null);
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
      setSavingSection(null);
    }
  };

  const onSubmitAdvanced = async (data: AdvancedFormData) => {
    if (!instance?.apikey || !instance?.id || !instanceId) {
      toast.error(t('instanceSettings.toast.tokenNotFound'));
      return;
    }

    try {
      setSavingSection("behavior");
      await instancesApi.updateAdvancedSettings(
        instance.id,
        instance.apikey,
        data
      );
      resetAdvanced({
        alwaysOnline: Boolean(data.alwaysOnline),
        rejectCall: Boolean(data.rejectCall),
        readMessages: Boolean(data.readMessages),
        ignoreGroups: Boolean(data.ignoreGroups),
        ignoreStatus: Boolean(data.ignoreStatus),
      });
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
      setSavingSection(null);
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

  const handleGeneratePairingCode = async () => {
    if (!instance?.apikey || !instanceId) {
      toast.error(t('instanceSettings.toast.tokenNotFound'));
      return;
    }

    if (instance.connected) {
      toast.error(t('instanceSettings.connection.alreadyConnected'));
      return;
    }

    let phone = pairPhone.trim();
    if (!phone) {
      toast.error(t('instanceSettings.connection.phoneRequired'));
      return;
    }
    if (!phone.startsWith('+')) {
      phone = `+${phone}`;
    }

    setIsPairing(true);
    setPairingCode(null);
    try {
      // Empty body: webhook/events/producers are already persisted from the
      // Webhook card; this only starts the session so pairing can begin.
      await instancesApi.connectInstance(instance.apikey, {});
      const pairData = await instancesApi.pairInstance(instance.apikey, {
        subscribe: instance.events ? parseEvents(instance.events) : [],
        phone,
      });
      setPairingCode(pairData.pairingCode);
      toast.success(t('instanceSettings.connection.codeGenerated'));
    } catch (error) {
      console.error('Erro ao gerar código de pareamento:', error);
      toast.error(
        error instanceof Error
          ? error.message
          : t('instanceSettings.connection.codeError')
      );
    } finally {
      setIsPairing(false);
    }
  };

  const handleCopyPairingCode = async () => {
    if (!pairingCode) return;
    try {
      await navigator.clipboard.writeText(pairingCode);
      setCopiedPairing(true);
      toast.success(t('instanceSettings.connection.codeCopied'));
      setTimeout(() => setCopiedPairing(false), 2000);
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
      setProxyForm({
        protocol: saved?.protocol || "http",
        host: saved?.host || "",
        port: saved?.port || "",
        username: saved?.username || "",
        password: "",
      });
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

  const handleSaveS3 = async () => {
    if (!instance) return;
    setS3Busy("save");
    try {
      const status = await instancesApi.setS3Config(instance.apikey || "", {
        ...s3Form,
        // Empty means "keep the stored secret" on the backend.
        secretKey: s3Form.secretKey || undefined,
      });
      setS3Form((form) => ({ ...form, ...status, secretKey: "" }));
      toast.success(t('instanceSettings.toast.s3Saved'));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t('instanceSettings.toast.s3SaveError'));
    } finally {
      setS3Busy(null);
    }
  };

  const handleTestS3 = async () => {
    if (!instance) return;
    setS3Busy("test");
    try {
      const result = await instancesApi.testS3Config(instance.apikey || "", s3Form);
      toast.success(t('instanceSettings.toast.s3TestOk', { bucket: result.bucket }));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t('instanceSettings.toast.s3TestError'));
    } finally {
      setS3Busy(null);
    }
  };

  const handleDeleteS3 = async () => {
    if (!instance) return;
    setS3Busy("delete");
    try {
      await instancesApi.deleteS3Config(instance.apikey || "");
      setS3Form(EMPTY_S3_CONFIG);
      toast.success(t('instanceSettings.toast.s3Removed'));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t('instanceSettings.toast.s3RemoveError'));
    } finally {
      setS3Busy(null);
    }
  };

  const handleSaveHmac = async () => {
    if (!instance) return;
    setHmacBusy("save");
    try {
      const status = await instancesApi.setHmacKey(instance.apikey || "", hmacKey.trim());
      setHmac(status);
      setHmacKey("");
      setHmacGenerated(null);
      toast.success(t('instanceSettings.toast.hmacSaved'));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t('instanceSettings.toast.hmacSaveError'));
    } finally {
      setHmacBusy(null);
    }
  };

  const handleGenerateHmac = async () => {
    if (!instance) return;
    setHmacBusy("generate");
    try {
      const result = await instancesApi.setHmacKey(instance.apikey || "", undefined, true);
      setHmac(result);
      setHmacGenerated(result.generatedKey || null);
      toast.success(t('instanceSettings.toast.hmacGenerated'));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t('instanceSettings.toast.hmacSaveError'));
    } finally {
      setHmacBusy(null);
    }
  };

  const handleDeleteHmac = async () => {
    if (!instance) return;
    setHmacBusy("delete");
    try {
      await instancesApi.deleteHmacKey(instance.apikey || "");
      setHmac((s) => (s ? { ...s, configured: false } : s));
      setHmacGenerated(null);
      toast.success(t('instanceSettings.toast.hmacRemoved'));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t('instanceSettings.toast.hmacRemoveError'));
    } finally {
      setHmacBusy(null);
    }
  };

  useEffect(() => {
    const root = scrollerRef.current;
    if (!root || typeof IntersectionObserver === "undefined") return;
    const nodes = SECTIONS.map((section) => document.getElementById(section.id)).filter(
      (node): node is HTMLElement => node !== null
    );
    if (nodes.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        const id = visible[0]?.target.id;
        if (id) setActiveSection(id);
      },
      { root, rootMargin: "-10% 0px -70% 0px", threshold: 0 }
    );
    nodes.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [instance, isLoading]);

  if (isLoading) return <InstanceSettingsSkeleton />;

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

  const eventsDirty = editedEvents !== null && !sameMembers(editedEvents, parsedEvents);
  const proxyDirty = proxyFormDirty(proxyForm, proxy);
  const webhookBusy = savingSection === "webhook";
  const behaviorBusy = savingSection === "behavior";
  const saving = savingSection !== null;
  const unsavedLabel = t("instanceSettings.unsaved");
  const showSection = (id: string) => {
    setActiveSection(id);
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

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
      <div ref={scrollerRef} className="flex-1 overflow-y-auto">
        <nav
          aria-label={t("instanceSettings.nav.label")}
          className="sticky top-0 z-10 border-b border-sidebar-border bg-background"
        >
          <div className="mx-auto flex max-w-4xl gap-1 overflow-x-auto px-4 py-2 sm:px-6">
            {SECTIONS.map((section) => {
              const dirty =
                (section.id === "webhooks" && webhookFormDirty) ||
                (section.id === "events" && eventsDirty) ||
                (section.id === "proxy" && proxyDirty) ||
                (section.id === "behavior" && behaviorDirty);
              const active = activeSection === section.id;
              return (
                <button
                  key={section.id}
                  type="button"
                  data-section={section.id}
                  data-unsaved={dirty ? "true" : "false"}
                  aria-current={active ? "true" : undefined}
                  onClick={() => showSection(section.id)}
                  className={cn(
                    "inline-flex shrink-0 items-center gap-1.5 rounded-full px-3 py-1 text-sm transition-colors",
                    active
                      ? "bg-primary/10 font-medium text-primary"
                      : "text-muted-foreground hover:bg-accent hover:text-foreground"
                  )}
                >
                  {t(section.labelKey)}
                  {dirty && (
                    <span className="h-1.5 w-1.5 rounded-full bg-amber-500" aria-hidden />
                  )}
                </button>
              );
            })}
          </div>
        </nav>
        <div className="mx-auto max-w-4xl space-y-6 p-4 sm:p-6">
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

          {instance.integration === 'WHATSAPP-BUSINESS' && (
            <CloudSettingsCard instance={instance} />
          )}

          {/* Connection Settings Card */}
          <section id="connection" className={sectionClass(false)}>
            <h2 className="mb-1 flex items-center gap-2 text-lg font-semibold text-foreground">
              <KeyRound className="h-5 w-5" />
              {t('instanceSettings.connection.title')}
            </h2>
            <p className="mb-4 text-xs text-muted-foreground">
              {t('instanceSettings.connection.description')}
            </p>

            <div className="space-y-4">
              <div>
                <label
                  htmlFor="pairPhone"
                  className="mb-1 block text-sm font-medium text-foreground"
                >
                  {t('instanceSettings.connection.phone')}
                </label>
                <div className="flex flex-col gap-2 sm:flex-row">
                  <input
                    id="pairPhone"
                    type="tel"
                    value={pairPhone}
                    onChange={(e) => setPairPhone(e.target.value)}
                    placeholder={t('instanceSettings.connection.phonePlaceholder')}
                    disabled={isPairing || instance.connected}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-60 sm:flex-1"
                  />
                  <Button
                    type="button"
                    onClick={handleGeneratePairingCode}
                    disabled={
                      isPairing || instance.connected || !pairPhone.trim()
                    }
                    className="gap-2 sm:w-auto"
                  >
                    {isPairing ? (
                      <RefreshCw className="h-4 w-4 animate-spin" />
                    ) : (
                      <KeyRound className="h-4 w-4" />
                    )}
                    {isPairing
                      ? t('instanceSettings.connection.generating')
                      : t('instanceSettings.connection.generate')}
                  </Button>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('instanceSettings.connection.phoneHelp')}
                </p>
              </div>

              {instance.connected && (
                <div className="rounded-md border border-sidebar-border bg-sidebar-accent/40 p-3 text-xs text-muted-foreground">
                  {t('instanceSettings.connection.connectedHint')}
                </div>
              )}

              {pairingCode && !instance.connected && (
                <div className="rounded-lg border border-primary/30 bg-primary/5 p-4">
                  <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {t('instanceSettings.connection.codeLabel')}
                  </p>
                  <div className="mt-2 flex items-center gap-2">
                    <p className="font-mono text-2xl font-semibold tracking-[0.2em] text-foreground">
                      {pairingCode}
                    </p>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      onClick={handleCopyPairingCode}
                      title={t('instanceSettings.connection.copyCode')}
                      aria-label={t('instanceSettings.connection.copyCode')}
                    >
                      {copiedPairing ? (
                        <Check className="h-4 w-4 text-green-500" />
                      ) : (
                        <Copy className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {t('instanceSettings.connection.codeHelp')}
                  </p>
                  <div className="mt-3 flex items-center gap-2 text-xs text-primary">
                    <span className="relative flex h-2 w-2">
                      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
                      <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
                    </span>
                    {t('instanceSettings.connection.waiting')}
                  </div>
                </div>
              )}
            </div>
          </section>

          {/* Webhook and events share one save: the API writes them together. */}
          <form onSubmit={handleSubmitWebhook(onSubmitWebhook)} className="space-y-6">
            <section id="webhooks" className={sectionClass(webhookFormDirty)}>
              <h2 className="mb-4 flex items-center gap-2 text-lg font-semibold text-foreground">
                {t('instanceSettings.webhook.title')}
                {webhookFormDirty && <UnsavedMark label={unsavedLabel} />}
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

                <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
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
                  <Button type="submit" disabled={saving || !webhookFormDirty} className="gap-2">
                    <Save className="h-4 w-4" />
                    {webhookBusy ? t('instanceSettings.action.saving') : t('instanceSettings.webhook.save')}
                  </Button>
                </div>
              </div>
            </section>

            <section id="events" className={sectionClass(eventsDirty)}>
              <h2 className="mb-1 flex items-center gap-2 text-lg font-semibold text-foreground">
                {t('instanceSettings.nav.events')}
                {eventsDirty && <UnsavedMark label={unsavedLabel} />}
              </h2>
              <p className="mb-4 text-xs text-muted-foreground">
                {t('instanceSettings.events.savedWithWebhook')}
              </p>
              <div className="space-y-4">
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

                <div className="flex justify-end">
                  <Button type="submit" disabled={saving || !eventsDirty} className="gap-2">
                    <Save className="h-4 w-4" />
                    {webhookBusy ? t('instanceSettings.action.saving') : t('instanceSettings.events.save')}
                  </Button>
                </div>
              </div>
            </section>
          </form>

          {/* Proxy Settings Card */}
          <section id="proxy" className={sectionClass(proxyDirty)}>
            <div className="mb-4 flex items-center justify-between gap-3">
              <h2 className="flex items-center gap-2 text-lg font-semibold text-foreground">
                <Network className="h-5 w-5" />
                {t('instanceSettings.proxy.title')}
                {proxyDirty && <UnsavedMark label={unsavedLabel} />}
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
                  onClick={() => setPendingConfirm("removeProxy")}
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
                  disabled={proxyBusy !== null || !proxyDirty}
                  className="gap-2"
                >
                  <Save className="h-4 w-4" />
                  {proxyBusy === "save" ? t('instanceSettings.action.saving') : t('instanceSettings.proxy.save')}
                </Button>
              </div>
            </div>
          </section>

          {/* Per-instance S3 */}
          <section id="storage" className={sectionClass(false)}>
            <h2 className="mb-1 text-lg font-semibold text-foreground">
              {t('instanceSettings.nav.storage')}
            </h2>
            <p className="mb-4 text-sm text-muted-foreground">
              {t('instanceSettings.s3.description')}
            </p>
            <div className="space-y-4">
              <label className="flex items-center gap-2 text-sm text-foreground">
                <input
                  type="checkbox"
                  checked={s3Form.enabled}
                  onChange={(e) => setS3Form({ ...s3Form, enabled: e.target.checked })}
                />
                {t('instanceSettings.s3.enabled')}
              </label>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <label className="text-sm">
                  <span className="font-medium">{t('instanceSettings.s3.endpoint')}</span>
                  <input
                    className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={s3Form.endpoint}
                    onChange={(e) => setS3Form({ ...s3Form, endpoint: e.target.value })}
                    placeholder="s3.amazonaws.com"
                  />
                </label>
                <label className="text-sm">
                  <span className="font-medium">{t('instanceSettings.s3.bucket')}</span>
                  <input
                    className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={s3Form.bucket}
                    onChange={(e) => setS3Form({ ...s3Form, bucket: e.target.value })}
                  />
                </label>
                <label className="text-sm">
                  <span className="font-medium">{t('instanceSettings.s3.region')}</span>
                  <input
                    className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={s3Form.region || ""}
                    onChange={(e) => setS3Form({ ...s3Form, region: e.target.value })}
                  />
                </label>
                <label className="text-sm">
                  <span className="font-medium">{t('instanceSettings.s3.accessKey')}</span>
                  <input
                    className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={s3Form.accessKey}
                    onChange={(e) => setS3Form({ ...s3Form, accessKey: e.target.value })}
                  />
                </label>
                <label className="text-sm">
                  <span className="font-medium">{t('instanceSettings.s3.secretKey')}</span>
                  <input
                    type="password"
                    className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={s3Form.secretKey || ""}
                    onChange={(e) => setS3Form({ ...s3Form, secretKey: e.target.value })}
                    placeholder={s3Form.secretKeySet ? t('instanceSettings.s3.secretStored') : ''}
                  />
                </label>
                <label className="text-sm">
                  <span className="font-medium">{t('instanceSettings.s3.delivery')}</span>
                  <select
                    className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={s3Form.mediaDelivery || "base64"}
                    onChange={(e) =>
                      setS3Form({ ...s3Form, mediaDelivery: e.target.value as S3Config['mediaDelivery'] })
                    }
                  >
                    <option value="base64">base64</option>
                    <option value="s3">s3</option>
                    <option value="both">both</option>
                  </select>
                </label>
                <label className="text-sm">
                  <span className="font-medium">{t('instanceSettings.s3.publicUrl')}</span>
                  <input
                    className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    value={s3Form.publicUrl || ""}
                    onChange={(e) => setS3Form({ ...s3Form, publicUrl: e.target.value })}
                  />
                </label>
                <label className="flex items-center gap-2 self-end text-sm text-foreground">
                  <input
                    type="checkbox"
                    checked={Boolean(s3Form.pathStyle)}
                    onChange={(e) => setS3Form({ ...s3Form, pathStyle: e.target.checked })}
                  />
                  {t('instanceSettings.s3.pathStyle')}
                </label>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" onClick={handleTestS3} disabled={s3Busy !== null}>
                  {s3Busy === "test" ? t('instanceSettings.s3.testing') : t('instanceSettings.s3.test')}
                </Button>
                <Button type="button" onClick={handleSaveS3} disabled={s3Busy !== null}>
                  {s3Busy === "save" ? t('instanceSettings.action.saving') : t('instanceSettings.s3.save')}
                </Button>
                <Button type="button" variant="ghost" onClick={handleDeleteS3} disabled={s3Busy !== null}>
                  {t('instanceSettings.s3.remove')}
                </Button>
              </div>
            </div>
          </section>

          {/* Webhook HMAC + health */}
          <section id="security" className={sectionClass(false)}>
            <h2 className="mb-1 text-lg font-semibold text-foreground">
              {t('instanceSettings.nav.security')}
            </h2>
            <p className="mb-4 text-sm text-muted-foreground">{t('instanceSettings.hmac.description')}</p>
            <div className="space-y-4">
              <p className="text-sm">
                {t('instanceSettings.hmac.status')}:{' '}
                <strong>
                  {hmac?.configured
                    ? t('instanceSettings.hmac.configured')
                    : t('instanceSettings.hmac.notConfigured')}
                </strong>
                {hmac?.globalFallback ? ` (${t('instanceSettings.hmac.globalFallback')})` : ''}
              </p>
              <div className="flex flex-col gap-2 sm:flex-row">
                <input
                  type="password"
                  className="min-w-0 flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm"
                  value={hmacKey}
                  onChange={(e) => setHmacKey(e.target.value)}
                  placeholder={t('instanceSettings.hmac.keyPlaceholder')}
                />
                <Button
                  type="button"
                  onClick={handleSaveHmac}
                  disabled={hmacBusy !== null || hmacKey.trim().length < 32}
                >
                  {hmacBusy === "save" ? t('instanceSettings.action.saving') : t('instanceSettings.hmac.save')}
                </Button>
                <Button type="button" variant="outline" onClick={handleGenerateHmac} disabled={hmacBusy !== null}>
                  {hmacBusy === "generate" ? t('instanceSettings.action.saving') : t('instanceSettings.hmac.generate')}
                </Button>
                <Button type="button" variant="ghost" onClick={handleDeleteHmac} disabled={hmacBusy !== null}>
                  {t('instanceSettings.hmac.remove')}
                </Button>
              </div>
              {hmacGenerated && (
                <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
                  <p className="mb-1 font-medium">{t('instanceSettings.hmac.generatedOnce')}</p>
                  <code className="break-all">{hmacGenerated}</code>
                </div>
              )}
              {health && (
                <div className="rounded-md border border-sidebar-border p-3 text-sm">
                  <p className="font-medium">
                    {t('instanceSettings.health.title')}: {health.verdict}
                  </p>
                  {health.reasons?.length > 0 && (
                    <p className="text-muted-foreground">{health.reasons.join(', ')}</p>
                  )}
                  {health.lastEventAt && (
                    <p className="text-muted-foreground">
                      {t('instanceSettings.health.lastEvent')}: {health.lastEventAt}
                    </p>
                  )}
                  {health.lastDisconnectReason && (
                    <p className="text-muted-foreground">{health.lastDisconnectReason}</p>
                  )}
                </div>
              )}
            </div>
          </section>

          {/* Advanced Settings Card */}
          <form onSubmit={handleSubmitAdvanced(onSubmitAdvanced)}>
            <section id="behavior" className={sectionClass(behaviorDirty)}>
              <h2 className="mb-4 flex items-center gap-2 text-lg font-semibold text-foreground">
                {t('instanceSettings.nav.behavior')}
                {behaviorDirty && <UnsavedMark label={unsavedLabel} />}
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
                  <Button type="submit" disabled={saving || !behaviorDirty} className="gap-2">
                    <Save className="h-4 w-4" />
                    {behaviorBusy ? t('instanceSettings.action.saving') : t('instanceSettings.behavior.save')}
                  </Button>
                </div>
              </div>
            </section>
          </form>

          {/* Danger Zone Card */}
          <section id="danger" className="scroll-mt-16 rounded-lg border border-destructive/50 bg-destructive/10 p-6">
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
                  onClick={() => setPendingConfirm("disconnect")}
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
                  onClick={() => setPendingConfirm("delete")}
                  className="gap-2"
                >
                  <Trash2 className="h-4 w-4" />
                  {t('instanceSettings.danger.delete')}
                </Button>
              </div>
            </div>
          </section>
        </div>
      </div>

      <ConfirmDialog
        open={pendingConfirm === "removeProxy"}
        destructive
        title={t('instanceSettings.confirm.removeProxyTitle')}
        description={t('instanceSettings.confirm.removeProxy')}
        confirmLabel={t('instanceSettings.proxy.remove')}
        cancelLabel={t('common.cancel')}
        onCancel={() => setPendingConfirm(null)}
        onConfirm={() => {
          setPendingConfirm(null);
          void handleDeleteProxy();
        }}
      />
      <ConfirmDialog
        open={pendingConfirm === "disconnect"}
        destructive
        title={t('instanceSettings.danger.disconnectTitle')}
        description={t('instanceSettings.confirm.disconnect', { name: instance?.instanceName ?? '' })}
        confirmLabel={t('instanceSettings.danger.disconnect')}
        cancelLabel={t('common.cancel')}
        onCancel={() => setPendingConfirm(null)}
        onConfirm={() => {
          setPendingConfirm(null);
          void handleDisconnect();
        }}
      />
      <ConfirmDialog
        open={pendingConfirm === "delete"}
        destructive
        title={t('instanceSettings.danger.deleteTitle')}
        description={t('instanceSettings.confirm.delete', { name: instance?.instanceName ?? '' })}
        confirmLabel={t('instanceSettings.danger.delete')}
        cancelLabel={t('common.cancel')}
        onCancel={() => setPendingConfirm(null)}
        onConfirm={() => {
          setPendingConfirm(null);
          void handleDelete();
        }}
      />
    </div>
  );
}
