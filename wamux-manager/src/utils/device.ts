/**
 * Human label for the platform whatsmeow reports for the paired phone
 * (store.Device.Platform, taken from the pair-success node). whatsmeow does not
 * expose a phone model, only this platform string.
 */
const LABELS: Record<string, string> = {
  android: "Android",
  ios: "iPhone (iOS)",
  smb_android: "WhatsApp Business (Android)",
  smb_ios: "WhatsApp Business (iOS)",
  web: "Web",
  windows: "Windows",
  macos: "macOS",
};

export function deviceLabel(platform?: string): string | undefined {
  if (!platform) return undefined;
  return LABELS[platform.toLowerCase()] || platform;
}
