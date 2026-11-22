import { useCallback, useEffect, useState } from 'react';
import type { ChangeEvent } from 'react';
import { Button, Card, CardContent, Input, Label, Badge } from '@/components/ui';
import { toast } from 'sonner';
import * as api from '@/services/api/instances';
import type { CloudDiagnostics, CloudTemplate, Instance } from '@/types/instance';

type Props = { instance: Instance };

const emptyForm = {
  appId: '',
  wabaId: '',
  phoneNumberId: '',
  businessId: '',
  accessToken: '',
  enforceWindow: true,
};

export default function CloudSettingsCard({ instance }: Props) {
  const token = instance.apikey || '';
  const [form, setForm] = useState(emptyForm);
  const [pin, setPin] = useState('');
  const [diagnostics, setDiagnostics] = useState<CloudDiagnostics | null>(null);
  const [templates, setTemplates] = useState<CloudTemplate[]>([]);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      const [creds, diag, tpls] = await Promise.all([
        api.getCloudCredentials(token).catch(() => null),
        api.getCloudDiagnostics(token).catch(() => null),
        api.listCloudTemplates(token).catch(() => []),
      ]);
      if (creds?.configured) {
        setForm({
          appId: creds.appId || '',
          wabaId: creds.wabaId || '',
          phoneNumberId: creds.phoneNumberId || '',
          businessId: creds.businessId || '',
          accessToken: '',
          enforceWindow: creds.enforceWindow !== false,
        });
      }
      setDiagnostics(diag);
      setTemplates(tpls);
    } catch (error) {
      console.error(error);
    }
  }, [token]);

  // The loading function owns the network fetch and its resulting state.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { void load(); }, [load]);

  const set = (key: keyof typeof form) => (e: ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [key]: e.target.value }));

  const save = async () => {
    setBusy('save');
    try {
      await api.setCloudCredentials(token, {
        appId: form.appId || undefined,
        wabaId: form.wabaId,
        phoneNumberId: form.phoneNumberId,
        businessId: form.businessId || undefined,
        accessToken: form.accessToken || undefined,
        enforceWindow: form.enforceWindow,
      });
      toast.success('Cloud credentials saved');
      await load();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not save Cloud credentials');
    } finally { setBusy(null); }
  };

  const verify = async () => {
    setBusy('verify');
    try {
      await api.verifyCloud(token);
      toast.success('Cloud API verified and app subscribed');
      await load();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Cloud API verification failed');
    } finally { setBusy(null); }
  };

  const register = async () => {
    setBusy('register');
    try {
      await api.registerCloudPhone(token, pin);
      setPin('');
      toast.success('Phone registered for Cloud API');
      await load();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Phone registration failed');
    } finally { setBusy(null); }
  };

  const sync = async () => {
    setBusy('sync');
    try {
      setTemplates(await api.syncCloudTemplates(token));
      toast.success('Templates synced');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Template sync failed');
    } finally { setBusy(null); }
  };

  return (
    <Card className="scroll-mt-16 border-sidebar-border bg-card">
      <CardContent className="space-y-4 p-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-lg font-semibold text-sidebar-foreground">WhatsApp Cloud API</h2>
            <p className="text-xs text-sidebar-foreground/60">
              Official Meta transport. Access tokens are encrypted and write-only.
            </p>
          </div>
          <div className="flex gap-2">
            {diagnostics?.phoneRegistered && <Badge className="bg-green-500/10 text-green-600">Registered</Badge>}
            {diagnostics?.subscribed && <Badge className="bg-blue-500/10 text-blue-600">Subscribed</Badge>}
            {diagnostics && !diagnostics.tokenValid && <Badge className="bg-red-500/10 text-red-600">Token invalid</Badge>}
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-2">
          <div className="space-y-1">
            <Label>Meta app ID</Label>
            <Input value={form.appId} onChange={set('appId')} placeholder="1234567890" />
          </div>
          <div className="space-y-1">
            <Label>WABA ID</Label>
            <Input value={form.wabaId} onChange={set('wabaId')} placeholder="9876543210" />
          </div>
          <div className="space-y-1">
            <Label>Phone number ID</Label>
            <Input value={form.phoneNumberId} onChange={set('phoneNumberId')} placeholder="1122334455" />
          </div>
          <div className="space-y-1">
            <Label>Business ID</Label>
            <Input value={form.businessId} onChange={set('businessId')} placeholder="5566778899" />
          </div>
          <div className="space-y-1 md:col-span-2">
            <Label>Access token {form.accessToken ? '' : '(unchanged if blank)'}</Label>
            <Input
              type="password"
              autoComplete="off"
              value={form.accessToken}
              onChange={set('accessToken')}
              placeholder="EAAG..."
            />
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button onClick={save} disabled={!!busy}>{busy === 'save' ? 'Saving…' : 'Save credentials'}</Button>
          <Button variant="outline" onClick={verify} disabled={!!busy}>{busy === 'verify' ? 'Verifying…' : 'Verify'}</Button>
          <Button variant="outline" onClick={sync} disabled={!!busy}>{busy === 'sync' ? 'Syncing…' : 'Sync templates'}</Button>
        </div>

        <div className="rounded-md border border-sidebar-border p-3">
          <Label>Register phone (six-digit PIN)</Label>
          <div className="mt-2 flex gap-2">
            <Input value={pin} onChange={(e) => setPin(e.target.value)} maxLength={6} placeholder="123456" />
            <Button onClick={register} disabled={!!busy || pin.length !== 6}>
              {busy === 'register' ? 'Registering…' : 'Register'}
            </Button>
          </div>
          <p className="mt-1 text-xs text-sidebar-foreground/60">
            The PIN is sent to Meta and never stored by WaMux.
          </p>
        </div>

        {diagnostics && (
          <div className="grid gap-2 rounded-md bg-sidebar-accent/30 p-3 text-xs md:grid-cols-2">
            <div>Display phone: {diagnostics.displayPhoneNumber || '—'}</div>
            <div>Verified name: {diagnostics.verifiedName || '—'}</div>
            <div>Quality: {diagnostics.qualityRating || '—'}</div>
            <div>Messaging limit: {diagnostics.messagingLimit || '—'}</div>
            <div>Templates: {diagnostics.templatesApproved ?? 0} approved / {diagnostics.templatesPending ?? 0} pending</div>
            <div>Last webhook: {diagnostics.lastWebhookAt || '—'}</div>
            {diagnostics.lastGraphError && <div className="md:col-span-2 text-red-500">{diagnostics.lastGraphError}</div>}
          </div>
        )}

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold">Templates</h3>
            <Badge variant="secondary">{templates.length}</Badge>
          </div>
          {templates.length === 0 ? (
            <p className="text-xs text-sidebar-foreground/60">No templates synced.</p>
          ) : (
            <div className="max-h-56 divide-y divide-sidebar-border overflow-auto rounded-md border border-sidebar-border">
              {templates.map((tpl) => (
                <div key={`${tpl.name}:${tpl.language}`} className="flex items-center justify-between gap-2 p-2 text-xs">
                  <div className="min-w-0">
                    <div className="truncate font-medium">{tpl.name}</div>
                    <div className="truncate text-sidebar-foreground/60">{tpl.language} · {tpl.category || '—'}</div>
                  </div>
                  <Badge variant="secondary">{tpl.status || 'UNKNOWN'}</Badge>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={async () => {
                      try {
                        await api.deleteCloudTemplate(token, tpl.name, tpl.language);
                        await load();
                        toast.success('Template deleted');
                      } catch (error) {
                        toast.error(error instanceof Error ? error.message : 'Could not delete template');
                      }
                    }}
                  >
                    Delete
                  </Button>
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
