import { useEffect, useState, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { ArrowLeft, Save, Trash2, Power, Eye, EyeOff, Copy, Check, Network, RefreshCw, Pencil, X } from "lucide-react";
import { Button } from "@evoapi/design-system";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import * as instancesApi from "@/services/api/instances";
import type { Instance, InstanceOverview, ProxyConfig, ProxyTestResult } from "@/types/instance";
import { deviceLabel } from "@/utils/device";
import useInstancesStore from "@/store/instancesStore";

const webhookSchema = z.object({
  webhookUrl: z.string().url("URL inválida").optional().or(z.literal("")),
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
  const { instanceId } = useParams<{ instanceId: string }>();
  const navigate = useNavigate();
  const [instance, setInstance] = useState<Instance | null>(null);
  const [overview, setOverview] = useState<InstanceOverview | null>(null);
  const [selectedEvents, setSelectedEvents] = useState<string[]>([]);
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
        toast.error("Erro ao carregar dados da instância");
      } finally {
        setIsLoading(false);
      }
    };

    loadInstance();
  }, [instanceId]);

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

    // Parse events string to array (e.g., "MESSAGE,QRCODE,CONNECTION" -> ["MESSAGE", "QRCODE", "CONNECTION"])
    if (instance.events) {
      const eventsArray = instance.events
        .split(",")
        .map((e) => e.trim())
        .filter(Boolean);
      setSelectedEvents(eventsArray);
    } else {
      setSelectedEvents([]);
    }

    isInitialized.current = true;
  }, [instance, resetWebhook, resetAdvanced]);

  const toggleEvent = (event: string) => {
    setSelectedEvents((prev) => {
      if (event === "ALL") {
        if (prev.includes("ALL")) {
          return [];
        }
        return ["ALL"];
      }

      if (prev.includes("ALL")) {
        return [event];
      }

      if (prev.includes(event)) {
        return prev.filter((e) => e !== event);
      }
      return [...prev, event];
    });
  };

  const onSubmitWebhook = async (data: WebhookFormData) => {
    if (!instance?.apikey || !instanceId) {
      toast.error("Token da instância não encontrado");
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
      toast.success("Configurações de webhook atualizadas!");

      // Refetch instance data
      const updatedInstance = await instancesApi.fetchInstance(instanceId);
      setInstance(updatedInstance);
    } catch (error) {
      console.error("Erro ao atualizar webhook:", error);
      toast.error(
        error instanceof Error ? error.message : "Erro ao atualizar webhook"
      );
    } finally {
      setIsSaving(false);
    }
  };

  const onSubmitAdvanced = async (data: AdvancedFormData) => {
    if (!instance?.apikey || !instance?.id || !instanceId) {
      toast.error("Token da instância não encontrado");
      return;
    }

    try {
      setIsSaving(true);
      await instancesApi.updateAdvancedSettings(
        instance.id,
        instance.apikey,
        data
      );
      toast.success("Configurações avançadas atualizadas!");

      // Refetch instance data
      const updatedInstance = await instancesApi.fetchInstance(instanceId);
      setInstance(updatedInstance);
    } catch (error) {
      console.error("Erro ao atualizar configurações:", error);
      toast.error(
        error instanceof Error
          ? error.message
          : "Erro ao atualizar configurações"
      );
    } finally {
      setIsSaving(false);
    }
  };

  const handleDisconnect = async () => {
    if (!instance?.apikey || !instanceId) {
      toast.error("Token da instância não encontrado");
      return;
    }

    try {
      toast.info(`Desconectando ${instance.instanceName}...`);
      await instancesApi.logoutInstance(instance.apikey);

      // Refetch instance data
      const updatedInstance = await instancesApi.fetchInstance(instanceId);
      setInstance(updatedInstance);

      toast.success(`${instance.instanceName} desconectada!`);
    } catch (error) {
      console.error("Erro ao desconectar instância:", error);
      toast.error(
        error instanceof Error ? error.message : "Erro ao desconectar instância"
      );
    }
  };

  const handleDelete = async () => {
    if (!instance?.id) {
      toast.error("ID da instância não encontrado");
      return;
    }

    const confirmed = window.confirm(
      `Tem certeza que deseja deletar a instância ${instance.instanceName}? Esta ação não pode ser desfeita.`
    );

    if (!confirmed) return;

    try {
      toast.info(`Deletando ${instance.instanceName}...`);
      await instancesApi.deleteInstance(instance.id);
      toast.success(`${instance.instanceName} deletada!`);
      navigate("/manager/instances");
    } catch (error) {
      console.error("Erro ao deletar instância:", error);
      toast.error(
        error instanceof Error ? error.message : "Erro ao deletar instância"
      );
    }
  };

  const handleCopyToken = async () => {
    if (!instance?.apikey) return;
    try {
      await navigator.clipboard.writeText(instance.apikey);
      setCopied(true);
      toast.success("Token copiado!");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Não foi possível copiar o token");
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
      toast.error("Informe um nome para a instância");
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
        useInstancesStore.getState().updateInstance(instance.instanceName, {
          instanceName: updated.instanceName,
          profileName: updated.profileName,
        });
      }
      setIsEditingName(false);
      setNameDraft("");
      toast.success("Nome da instância atualizado!");
    } catch (error) {
      console.error("Erro ao renomear instância:", error);
      toast.error(
        error instanceof Error ? error.message : "Erro ao renomear instância"
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
      toast.error("Informe host e porta do proxy");
      return;
    }
    setProxyBusy("save");
    try {
      await instancesApi.setProxy(instanceId, proxyForm);
      const saved = await instancesApi.getProxy(instanceId);
      setProxy(saved);
      toast.success("Proxy salvo!");
    } catch (e) {
      toast.error(proxyErrorMessage(e, "Erro ao salvar proxy"));
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
      if (result.ok) toast.success("Proxy respondeu");
      else toast.error(result.error || "Proxy não respondeu");
    } catch (e) {
      toast.error(proxyErrorMessage(e, "Erro ao testar proxy"));
    } finally {
      setProxyBusy(null);
    }
  };

  const handleReconnectProxy = async () => {
    if (!instanceId) return;
    setProxyBusy("reconnect");
    try {
      await instancesApi.reconnectProxy(instanceId);
      toast.success("Reconectando através do proxy...");
    } catch (e) {
      toast.error(proxyErrorMessage(e, "Erro ao reconectar pelo proxy"));
    } finally {
      setProxyBusy(null);
    }
  };

  const handleDeleteProxy = async () => {
    if (!instanceId) return;
    if (!window.confirm("Remover a configuração de proxy desta instância?")) return;
    setProxyBusy("delete");
    try {
      await instancesApi.deleteProxy(instanceId);
      setProxy(null);
      setProxyForm(emptyProxy);
      setProxyTest(null);
      toast.success("Proxy removido!");
    } catch (e) {
      toast.error(proxyErrorMessage(e, "Erro ao remover proxy"));
    } finally {
      setProxyBusy(null);
    }
  };

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <p className="text-muted-foreground">Carregando...</p>
        </div>
      </div>
    );
  }

  if (!instance) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-semibold text-foreground mb-2">
            Instância não encontrada
          </h2>
          <p className="text-muted-foreground mb-4">
            A instância "{instanceId}" não foi encontrada.
          </p>
          <Button onClick={() => navigate("/manager/instances")}>
            <ArrowLeft className="h-4 w-4 mr-2" />
            Voltar para Instâncias
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
              className="text-sidebar-foreground hover:bg-sidebar-accent"
            >
              <ArrowLeft className="h-5 w-5" />
            </Button>
            <div>
              <h1 className="text-2xl font-bold text-foreground">
                Configurações
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
              Informações da Instância
            </h2>
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-sm font-medium text-foreground">
                    Nome da Instância
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
                        title="Salvar nome"
                      >
                        <Check className="h-4 w-4" />
                      </Button>
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        onClick={cancelEditName}
                        disabled={isRenaming}
                        title="Cancelar"
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
                        title="Editar nome"
                      >
                        <Pencil size={16} />
                      </button>
                    </div>
                  )}
                </div>
                <div>
                  <label className="text-sm font-medium text-foreground">
                    Token da Instância
                  </label>
                  <div className="mt-1 flex items-center gap-2">
                    <p className="text-sm text-muted-foreground font-mono">
                      {showToken ? (instance.apikey || '') : '•'.repeat((instance.apikey || '').length)}
                    </p>
                    <button
                      type="button"
                      onClick={() => setShowToken(!showToken)}
                      className="text-muted-foreground hover:text-foreground transition-colors"
                      title={showToken ? "Ocultar token" : "Mostrar token"}
                    >
                      {showToken ? <EyeOff size={18} /> : <Eye size={18} />}
                    </button>
                    <button
                      type="button"
                      onClick={handleCopyToken}
                      className="text-muted-foreground hover:text-foreground transition-colors"
                      title="Copiar token"
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
                    Status
                  </label>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {instance.status === "open" ? "Conectado" : "Desconectado"}
                  </p>
                </div>
                {instance.owner && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      Número
                    </label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {instance.owner}
                    </p>
                  </div>
                )}
                {instance.profileName && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      Nome do Perfil
                    </label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {instance.profileName}
                    </p>
                  </div>
                )}
                {deviceLabel(overview?.platform) && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      Dispositivo
                    </label>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {deviceLabel(overview?.platform)}
                    </p>
                  </div>
                )}
                {overview?.businessName && (
                  <div>
                    <label className="text-sm font-medium text-foreground">
                      Nome comercial
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
                Configurações de Webhook
              </h2>
              <div className="space-y-4">
                <div>
                  <label
                    htmlFor="webhookUrl"
                    className="block text-sm font-medium text-foreground mb-1"
                  >
                    URL do Webhook
                  </label>
                  <input
                    id="webhookUrl"
                    type="url"
                    placeholder="https://seu-servidor.com/webhook"
                    {...registerWebhook("webhookUrl")}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                  {webhookErrors.webhookUrl && (
                    <p className="mt-1 text-sm text-destructive">
                      {webhookErrors.webhookUrl.message}
                    </p>
                  )}
                  <p className="mt-1 text-xs text-muted-foreground">
                    URL que receberá os eventos do WhatsApp
                  </p>
                </div>

                {/* Events Selection */}
                <div>
                  <label className="block text-sm font-medium text-foreground mb-2">
                    Eventos para Webhook
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
                          ALL
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
                            {event}
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
                      <option value="">Padrão</option>
                      <option value="enabled">Habilitado</option>
                      <option value="disabled">Desabilitado</option>
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
                      <option value="">Padrão</option>
                      <option value="enabled">Habilitado</option>
                      <option value="disabled">Desabilitado</option>
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
                      <option value="">Padrão</option>
                      <option value="enabled">Habilitado</option>
                      <option value="disabled">Desabilitado</option>
                    </select>
                  </div>
                </div>

                <div className="flex justify-end">
                  <Button type="submit" disabled={isSaving} className="gap-2">
                    <Save className="h-4 w-4" />
                    {isSaving ? "Salvando..." : "Salvar Webhook"}
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
                Proxy
              </h2>
              <span
                className={`rounded-full border px-2 py-0.5 text-xs ${
                  proxy
                    ? "border-green-500/40 bg-green-500/10 text-green-500"
                    : "border-sidebar-border text-muted-foreground"
                }`}
              >
                {proxy ? "Configurado" : "Não configurado"}
              </span>
            </div>

            <div className="space-y-4">
              <p className="text-xs text-muted-foreground">
                Roteia a conexão WhatsApp da instância por um proxy. Salvar
                aplica no próximo connect/reconnect.
              </p>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    Protocolo
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
                    Host
                  </label>
                  <input
                    type="text"
                    placeholder="127.0.0.1"
                    value={proxyForm.host}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, host: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    Porta
                  </label>
                  <input
                    type="text"
                    placeholder="8080"
                    value={proxyForm.port}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, port: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-foreground">
                    Usuário <span className="text-muted-foreground">(opcional)</span>
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
                    Senha <span className="text-muted-foreground">(opcional)</span>
                  </label>
                  <input
                    type="password"
                    placeholder={proxy?.hasPassword ? "•••••• (inalterada)" : "opcional"}
                    value={proxyForm.password}
                    onChange={(e) =>
                      setProxyForm({ ...proxyForm, password: e.target.value })
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    A senha salva nunca é exibida. Deixe em branco para manter a
                    atual.
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
                        Proxy funcionando{proxyTest.protocol ? ` (${proxyTest.protocol})` : ""}
                      </p>
                      <p>
                        IP de saída: <span className="font-mono">{proxyTest.ip}</span>{" "}
                        {proxyTest.anonymous ? "(anônimo)" : "(não anônimo)"}
                      </p>
                      <p>
                        WhatsApp acessível:{" "}
                        {proxyTest.whatsappReachable ? "sim" : "não"}
                      </p>
                      {proxyTest.latencyMs !== undefined && (
                        <p>Latência: {proxyTest.latencyMs} ms</p>
                      )}
                    </div>
                  ) : (
                    <div className="space-y-1">
                      <p className="font-medium">Proxy falhou</p>
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
                  Remover
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleReconnectProxy}
                  disabled={!proxy || proxyBusy !== null}
                  className="gap-2"
                >
                  <RefreshCw className="h-4 w-4" />
                  Reconectar
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleTestProxy}
                  disabled={proxyBusy !== null}
                  className="gap-2"
                >
                  <Network className="h-4 w-4" />
                  {proxyBusy === "test" ? "Testando..." : "Testar"}
                </Button>
                <Button
                  type="button"
                  onClick={handleSaveProxy}
                  disabled={proxyBusy !== null}
                  className="gap-2"
                >
                  <Save className="h-4 w-4" />
                  {proxyBusy === "save" ? "Salvando..." : "Salvar Proxy"}
                </Button>
              </div>
            </div>
          </div>

          {/* Advanced Settings Card */}
          <form onSubmit={handleSubmitAdvanced(onSubmitAdvanced)}>
            <div className="rounded-lg border border-sidebar-border bg-card p-6">
              <h2 className="text-lg font-semibold text-foreground mb-4">
                Configurações Avançadas
              </h2>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <label
                      htmlFor="alwaysOnline"
                      className="text-sm font-medium text-foreground cursor-pointer"
                    >
                      Always Online
                    </label>
                    <p className="text-xs text-muted-foreground">
                      Manter sempre online no WhatsApp
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
                      Reject Call
                    </label>
                    <p className="text-xs text-muted-foreground">
                      Rejeitar chamadas automaticamente
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
                      Read Messages
                    </label>
                    <p className="text-xs text-muted-foreground">
                      Marcar mensagens como lidas
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
                      Ignore Groups
                    </label>
                    <p className="text-xs text-muted-foreground">
                      Ignorar mensagens de grupos
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
                      Ignore Status
                    </label>
                    <p className="text-xs text-muted-foreground">
                      Ignorar atualizações de status
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
                    {isSaving ? "Salvando..." : "Salvar Avançadas"}
                  </Button>
                </div>
              </div>
            </div>
          </form>

          {/* Danger Zone Card */}
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-6">
            <h2 className="text-lg font-semibold text-destructive mb-4">
              Zona de Perigo
            </h2>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-medium text-foreground">
                    Desconectar Instância
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    Desconecta a instância do WhatsApp
                  </p>
                </div>
                <Button
                  variant="destructive"
                  onClick={handleDisconnect}
                  className="gap-2"
                >
                  <Power className="h-4 w-4" />
                  Desconectar
                </Button>
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-medium text-foreground">
                    Deletar Instância
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    Remove permanentemente esta instância
                  </p>
                </div>
                <Button
                  variant="destructive"
                  onClick={handleDelete}
                  className="gap-2"
                >
                  <Trash2 className="h-4 w-4" />
                  Deletar
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
