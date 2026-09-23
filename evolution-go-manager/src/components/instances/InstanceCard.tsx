/**
 * Instance Card Component
 * Displays an instance as a card with status and actions
 */

import { Button, Card, CardContent, Badge } from "@evoapi/design-system";
import {
  Settings,
  Trash2,
  Power,
  PowerOff,
  MessageSquare,
  FlaskConical,
  Users,
  Mail,
  Smartphone,
} from "lucide-react";
import type { Instance, InstanceOverview } from "@/types/instance";
import { deviceLabel } from "@/utils/device";

type InstanceCardProps = {
  instance: Instance;
  overview?: InstanceOverview;
  isDeleting?: string | null;
  onSettings: (instance: Instance) => void;
  onDelete: (instance: Instance) => void;
  onConnect: (instance: Instance) => void;
  onDisconnect: (instance: Instance) => void;
  onSendMessage?: (instance: Instance) => void;
  onTestMessage?: (instance: Instance) => void;
};

const getStatusBadge = (status: string) => {
  if (status === "open") {
    return (
      <Badge className="bg-green-500/10 text-green-500 hover:bg-green-500/20">
        Conectado
      </Badge>
    );
  }

  return (
    <Badge className="bg-red-500/10 text-red-500 hover:bg-red-500/20">
      Desconectado
    </Badge>
  );
};

/** Two-letter fallback when the account has no profile picture. */
const initials = (name?: string) => {
  const n = (name || "").trim();
  if (!n) return "?";
  const parts = n.split(/\s+/);
  return (
    (parts[0][0] || "") + (parts.length > 1 ? parts[parts.length - 1][0] : "")
  ).toUpperCase();
};

const formatCount = (value?: number) =>
  value === undefined || value === null ? "—" : value.toLocaleString("pt-BR");

export default function InstanceCard({
  instance,
  overview,
  isDeleting,
  onSettings,
  onDelete,
  onConnect,
  onDisconnect,
  onSendMessage,
  onTestMessage,
}: InstanceCardProps) {
  const isConnected = instance.status === "open";
  const displayName =
    overview?.profileName || instance.profileName || instance.instanceName;

  return (
    <Card className="group relative bg-sidebar border-sidebar-border hover:bg-sidebar-accent/30 transition-all duration-300 hover:shadow-lg hover:shadow-black/10 overflow-hidden">
      <CardContent className="p-0">
        {/* Header with avatar, name and status */}
        <div className="flex items-center gap-3 p-4 border-b border-sidebar-border">
          <div className="flex-shrink-0">
            <div className="flex h-14 w-14 items-center justify-center overflow-hidden rounded-full border border-sidebar-border bg-sidebar-accent/40 text-sm font-semibold text-sidebar-foreground/70">
              {overview?.profilePicUrl ? (
                <img
                  src={overview.profilePicUrl}
                  alt={displayName}
                  className="h-14 w-14 rounded-full object-cover"
                  referrerPolicy="no-referrer"
                  onError={(e) => {
                    const target = e.target as HTMLImageElement;
                    target.style.display = "none";
                  }}
                />
              ) : (
                initials(displayName)
              )}
            </div>
          </div>

          <div className="flex-1 min-w-0">
            <h3 className="font-semibold text-base truncate text-sidebar-foreground">
              {displayName}
            </h3>
            <p className="text-xs text-sidebar-foreground/60 truncate">
              {instance.instanceName}
            </p>
          </div>

          <div className="flex-shrink-0">{getStatusBadge(instance.status)}</div>
        </div>

        {/* Details section */}
        <div className="px-4 py-3 text-xs text-sidebar-foreground/70 space-y-1">
          <div className="flex items-center justify-between">
            <span>Status</span>
            <span className="font-mono">{instance.status}</span>
          </div>
          {instance.owner && (
            <div className="flex items-center justify-between">
              <span>Proprietário</span>
              <span className="font-mono truncate ml-2 max-w-[150px]">
                {instance.owner}
              </span>
            </div>
          )}
          {deviceLabel(overview?.platform) && (
            <div className="flex items-center justify-between">
              <span className="inline-flex items-center gap-1">
                <Smartphone className="h-3 w-3" /> Dispositivo
              </span>
              <span className="truncate ml-2 max-w-[150px]">
                {deviceLabel(overview?.platform)}
              </span>
            </div>
          )}

          {/* Counts from GET /instance/overview/:id (connected instances only) */}
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1">
              <Users className="h-3 w-3" /> Contatos
            </span>
            <span className="font-mono">
              {isConnected ? formatCount(overview?.contactsCount) : "—"}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1">
              <MessageSquare className="h-3 w-3" /> Conversas
            </span>
            <span className="font-mono">
              {isConnected ? formatCount(overview?.chatsCount) : "—"}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1">
              <Mail className="h-3 w-3" /> Mensagens
            </span>
            <span className="font-mono">
              {isConnected ? formatCount(overview?.messagesCount) : "—"}
            </span>
          </div>
        </div>

        {/* Action buttons - hover effect */}
        <div className="flex border-t border-sidebar-border opacity-0 group-hover:opacity-100 transition-opacity duration-200">
          {/* Connect/Disconnect Button */}
          {!isConnected && (
            <Button
              variant="ghost"
              className="flex-1 rounded-none h-12 text-green-500 hover:text-green-400 hover:bg-green-500/10"
              onClick={() => onConnect(instance)}
            >
              <Power className="h-4 w-4 mr-2" />
              Conectar
            </Button>
          )}

          {isConnected && (
            <Button
              variant="ghost"
              className="flex-1 rounded-none h-12 text-yellow-500 hover:text-yellow-400 hover:bg-yellow-500/10"
              onClick={() => onDisconnect(instance)}
            >
              <PowerOff className="h-4 w-4 mr-2" />
              Desconectar
            </Button>
          )}

          <div className="w-px bg-sidebar-border" />

          {/* Send Message Button - only show if connected */}
          {isConnected && onSendMessage && (
            <>
              <Button
                variant="ghost"
                className="rounded-none h-12 px-4 text-blue-500 hover:text-blue-400 hover:bg-blue-500/10"
                onClick={() => onSendMessage(instance)}
                title="Enviar mensagem de texto"
              >
                <MessageSquare className="h-4 w-4" />
              </Button>
              <div className="w-px bg-sidebar-border" />
            </>
          )}

          {/* Test Interactive Messages Button - only show if connected */}
          {isConnected && onTestMessage && (
            <>
              <Button
                variant="ghost"
                className="rounded-none h-12 px-4 text-purple-500 hover:text-purple-400 hover:bg-purple-500/10"
                onClick={() => onTestMessage(instance)}
                title="Testar botoes, lista e carrossel"
              >
                <FlaskConical className="h-4 w-4" />
              </Button>
              <div className="w-px bg-sidebar-border" />
            </>
          )}

          {/* Settings Button */}
          <Button
            variant="ghost"
            className="rounded-none h-12 px-4 text-gray-500 hover:text-gray-300 hover:bg-gray-500/10"
            onClick={() => onSettings(instance)}
          >
            <Settings className="h-4 w-4" />
          </Button>

          <div className="w-px bg-sidebar-border" />

          {/* Delete Button */}
          <Button
            variant="ghost"
            className="rounded-none h-12 px-4 text-red-500 hover:text-red-400 hover:bg-red-500/10"
            disabled={isDeleting === instance.instanceName}
            onClick={() => onDelete(instance)}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
