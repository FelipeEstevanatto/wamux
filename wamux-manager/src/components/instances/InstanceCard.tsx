/**
 * Instance Card Component
 * Displays an instance as a card with status and actions
 */

import { Link } from "react-router-dom";
import { Button, Card, CardContent, Badge } from "@/components/ui";
import {
  Settings,
  Trash2,
  Power,
  PowerOff,
  MessageSquare,
  Users,
  Mail,
  Smartphone,
  Link2,
  Tag,
  Loader2,
} from "lucide-react";
import type { Instance, InstanceOverview } from "@/types/instance";
import { deviceLabel } from "@/utils/device";
import { prefetchRoute } from "@/utils/prefetch";
import { useI18n } from "@/i18n/I18nContext";

type InstanceCardProps = {
  instance: Instance;
  overview?: InstanceOverview;
  isDeleting?: string | null;
  isConnecting?: boolean;
  onSettings: (instance: Instance) => void;
  onDelete: (instance: Instance) => void;
  onConnect: (instance: Instance) => void;
  onDisconnect: (instance: Instance) => void;
  onOpen: (instance: Instance) => void;
  onSendMessage?: (instance: Instance) => void;
};

const getStatusBadge = (status: string, t: (key: string) => string) => {
  if (status === "open") {
    return (
      <Badge className="bg-green-500/10 text-green-500 hover:bg-green-500/20">
        {t("instances.connected")}
      </Badge>
    );
  }

  return (
    <Badge className="bg-red-500/10 text-red-500 hover:bg-red-500/20">
      {t("instances.disconnected")}
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

const formatCount = (value: number | undefined, locale: string) =>
  value === undefined || value === null ? "—" : value.toLocaleString(locale);

export default function InstanceCard({
  instance,
  overview,
  isDeleting,
  isConnecting,
  onSettings,
  onDelete,
  onConnect,
  onDisconnect,
  onOpen,
  onSendMessage,
}: InstanceCardProps) {
  const { t, locale } = useI18n();
  const messagesTo = `/manager/messages?instance=${encodeURIComponent(instance.id)}`;
  const isConnected = instance.status === "open";
  const displayName =
    overview?.profileName || instance.profileName || instance.instanceName;

  return (
    <Card
      className="relative cursor-pointer overflow-hidden border-sidebar-border bg-sidebar transition-colors hover:bg-sidebar-accent/30"
      onClick={() => onOpen(instance)}
      onMouseEnter={() => prefetchRoute("/manager/messages")}
    >
      <CardContent className="flex flex-1 flex-col p-0">
        {/* Header with avatar, name and status. The name is the keyboard path
            into this instance's messages; the rest of the card does the same. */}
        <div className="flex items-center gap-3 border-b border-sidebar-border p-4">
          <Link
            to={messagesTo}
            onClick={(e) => e.stopPropagation()}
            aria-label={t("instances.openMessages", { name: displayName })}
            className="flex min-w-0 flex-1 items-center gap-3 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
          <div className="shrink-0">
            <div className="flex h-14 w-14 items-center justify-center overflow-hidden rounded-full border border-sidebar-border bg-sidebar-accent/40 text-sm font-semibold text-sidebar-foreground/70">
              {overview?.profilePicUrl ? (
                <img
                  src={overview.profilePicUrl}
                  alt={displayName}
                  className="h-14 w-14 rounded-full object-cover"
                  referrerPolicy="no-referrer"
                  loading="lazy"
                  decoding="async"
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

          <div className="min-w-0 flex-1">
            <h3 className="truncate text-base font-semibold text-sidebar-foreground">
              {displayName}
            </h3>
            <p className="truncate text-xs text-sidebar-foreground/60">
              {instance.instanceName}
            </p>
          </div>
          </Link>

          <div className="shrink-0 flex items-center gap-2">
            {instance.integration === 'WHATSAPP-BUSINESS' && (
              <Badge className="bg-blue-500/10 text-blue-500 hover:bg-blue-500/20">
                Cloud API
              </Badge>
            )}
            {getStatusBadge(instance.status, t)}
          </div>
        </div>

        {/* Details section */}
        <div className="px-4 py-3 text-xs text-sidebar-foreground/70 space-y-1">
          <div className="flex items-center justify-between">
            <span>{t("instances.status")}</span>
            <span className="font-mono">{instance.status}</span>
          </div>
          {instance.owner && (
            <div className="flex items-center justify-between">
              <span>{t("instances.owner")}</span>
              <span className="font-mono truncate ml-2 max-w-[150px]">
                {instance.owner}
              </span>
            </div>
          )}
          {deviceLabel(overview?.platform) && (
            <div className="flex items-center justify-between">
              <span className="inline-flex items-center gap-1">
                <Smartphone className="h-3 w-3" /> {t("instances.device")}
              </span>
              <span className="truncate ml-2 max-w-[150px]">
                {deviceLabel(overview?.platform)}
              </span>
            </div>
          )}
          {instance.osName && (
            <div className="flex items-center justify-between">
              <span className="inline-flex items-center gap-1">
                <Link2 className="h-3 w-3" /> {t("instances.linkedDevice")}
              </span>
              <span
                className="truncate ml-2 max-w-[150px]"
                title={t("instances.linkedDeviceTitle")}
              >
                {instance.osName}
              </span>
            </div>
          )}
          {instance.clientName && (
            <div className="flex items-center justify-between">
              <span className="inline-flex items-center gap-1">
                <Tag className="h-3 w-3" /> {t("instances.client")}
              </span>
              <span className="truncate ml-2 max-w-[150px]">
                {instance.clientName}
              </span>
            </div>
          )}

          {/* Counts from GET /instance/overview/:id (connected instances only) */}
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1">
              <Users className="h-3 w-3" /> {t("instances.contacts")}
            </span>
            <span className="font-mono">
              {isConnected ? formatCount(overview?.contactsCount, locale) : "—"}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1">
              <MessageSquare className="h-3 w-3" /> {t("instances.chats")}
            </span>
            <span className="font-mono">
              {isConnected ? formatCount(overview?.chatsCount, locale) : "—"}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1">
              <Mail className="h-3 w-3" /> {t("instances.messages")}
            </span>
            <span className="font-mono">
              {isConnected ? formatCount(overview?.messagesCount, locale) : "—"}
            </span>
          </div>
        </div>

        {/* Actions stay visible. Desconectar is a compact labelled button, so
            the icons to its right are never pushed out of the card. Clicks
            here must not also open the inbox. */}
        <div
          className="mt-auto flex w-full items-stretch border-t border-sidebar-border"
          onClick={(e) => e.stopPropagation()}
        >
          {/* Connect/Disconnect Button */}
          {!isConnected && (
            <Button
              variant="ghost"
              className="h-12 shrink-0 rounded-none px-3 text-green-500 hover:bg-green-500/10 hover:text-green-400 sm:px-4"
              onClick={() => onConnect(instance)}
              disabled={isConnecting}
              title={t("instances.connectTitle")}
            >
              {isConnecting ? (
                <Loader2 className="mr-2 h-4 w-4 shrink-0 animate-spin" />
              ) : (
                <Power className="mr-2 h-4 w-4 shrink-0" />
              )}
              {isConnecting
                ? t("instances.connectingButton")
                : t("instances.connect")}
            </Button>
          )}

          {isConnected && (
            <Button
              variant="ghost"
              className="h-12 shrink-0 rounded-none px-3 text-yellow-500 hover:bg-yellow-500/10 hover:text-yellow-400 sm:px-4"
              onClick={() => onDisconnect(instance)}
              title={t("instances.disconnectTitle")}
            >
              <PowerOff className="mr-2 h-4 w-4 shrink-0" />
              {t("instances.disconnect")}
            </Button>
          )}

          <div className="w-px shrink-0 bg-sidebar-border" />

          {/* Send Message Button - only show if connected */}
          {isConnected && onSendMessage && (
            <>
              <Button
                variant="ghost"
                className="h-12 shrink-0 rounded-none px-3 text-blue-500 hover:bg-blue-500/10 hover:text-blue-400"
                onClick={() => onSendMessage(instance)}
                title={t("instances.sendMessage")}
                aria-label={t("instances.sendMessage")}
              >
                <MessageSquare className="h-4 w-4" />
              </Button>
              <div className="w-px shrink-0 bg-sidebar-border" />
            </>
          )}

          {/* Settings Button */}
          <Button
            variant="ghost"
            className="h-12 shrink-0 rounded-none px-3 text-gray-500 hover:bg-gray-500/10 hover:text-gray-300"
            onClick={() => onSettings(instance)}
            title={t("instances.settings")}
            aria-label={t("instances.settings")}
          >
            <Settings className="h-4 w-4" />
          </Button>

          <div className="w-px shrink-0 bg-sidebar-border" />

          {/* Spacer keeps the delete action pinned to the right edge. */}
          <div className="flex-1" />

          {/* Delete Button */}
          <Button
            variant="ghost"
            className="h-12 shrink-0 rounded-none px-3 text-red-500 hover:bg-red-500/10 hover:text-red-400"
            disabled={isDeleting === instance.instanceName}
            onClick={() => onDelete(instance)}
            title={t("instances.deleteTitle")}
            aria-label={t("instances.deleteTitle")}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
