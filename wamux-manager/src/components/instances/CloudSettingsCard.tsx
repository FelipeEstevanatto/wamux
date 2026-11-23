import { useCallback, useEffect, useState } from 'react';
import type { ChangeEvent } from 'react';
import { Button, Card, CardContent, ConfirmDialog, Input, Label, Badge } from '@/components/ui';
import { toast } from 'sonner';
import * as api from '@/services/api/instances';
import type { CloudAccountEvent, CloudDiagnostics, CloudTemplate, EmbeddedAsset, Instance } from '@/types/instance';

type Props = { instance: Instance };
type Tab = 'overview' | 'credentials' | 'phone' | 'templates' | 'events' | 'diagnostics';

const emptyForm = {
  appId: '',
  wabaId: '',
  phoneNumberId: '',
  businessId: '',
  accessToken: '',
  enforceWindow: true,
};

const emptyTemplate = { name: '', language: 'en_US', category: 'UTILITY', header: '', body: '', footer: '', buttons: '' };

export default function CloudSettingsCard({ instance }: Props) {
  const token = instance.apikey || '';
  const [tab, setTab] = useState<Tab>('overview');
  const [form, setForm] = useState(emptyForm);
  const [pin, setPin] = useState('');
  const [diagnostics, setDiagnostics] = useState<CloudDiagnostics | null>(null);
  const [templates, setTemplates] = useState<CloudTemplate[]>([]);
  const [events, setEvents] = useState<CloudAccountEvent[]>([]);
  const [eventField, setEventField] = useState('');
  const [templateForm, setTemplateForm] = useState(emptyTemplate);
  const [editingTemplate, setEditingTemplate] = useState<CloudTemplate | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CloudTemplate | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [embeddedState, setEmbeddedState] = useState('');
  const [embeddedAssets, setEmbeddedAssets] = useState<EmbeddedAsset[]>([]);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      const [creds, diag, tpls, evts] = await Promise.all([
        api.getCloudCredentials(token).catch(() => null),
        api.getCloudDiagnostics(token).catch(() => null),
        api.listCloudTemplates(token).catch(() => []),
        api.listCloudAccountEvents(token, { limit: 100 }).catch(() => []),
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
      setEvents(evts);
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

  const deregister = async () => {
    setBusy('deregister');
    try {
      await api.deregisterCloudPhone(token);
      toast.success('Phone deregistered');
      await load();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Phone deregistration failed');
    } finally { setBusy(null); }
  };

  const startEmbedded = async () => {
    setBusy('embedded');
    try {
      const start = await api.startEmbeddedSignup(token);
      setEmbeddedState(start.state);
      setEmbeddedAssets([]);
      window.open(start.url, '_blank', 'noopener,noreferrer');
      toast.success('Complete the Meta flow, then press “Load assets”');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not start Embedded Signup');
    } finally { setBusy(null); }
  };

  const loadEmbeddedAssets = async () => {
    if (!embeddedState) return;
    setBusy('embedded-assets');
    try {
      setEmbeddedAssets(await api.listEmbeddedAssets(token, embeddedState));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not load Embedded Signup assets');
    } finally { setBusy(null); }
  };

  const chooseEmbeddedAsset = async (asset: EmbeddedAsset) => {
    setBusy('embedded-select');
    try {
      await api.selectEmbeddedAsset(token, embeddedState, asset.wabaId, asset.phoneNumberId);
      setEmbeddedAssets([]);
      setEmbeddedState('');
      toast.success('Embedded Signup connected');
      await load();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not select the account');
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

  const loadEvents = async (field = eventField) => {
    setBusy('events');
    try {
      setEvents(await api.listCloudAccountEvents(token, { field: field || undefined, limit: 100 }));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not load account events');
    } finally { setBusy(null); }
  };

  const saveTemplate = async () => {
    if (!templateForm.name.trim() || !templateForm.language.trim()) {
      toast.error('Template name and language are required');
      return;
    }
    if (!editingTemplate && !templateForm.body.trim()) {
      toast.error('A body component is required for a new template');
      return;
    }
    setBusy('template');
    try {
      const components: Array<Record<string, unknown>> = [];
      if (templateForm.header.trim()) components.push({ type: 'HEADER', format: 'TEXT', text: templateForm.header.trim() });
      if (templateForm.body.trim()) components.push({ type: 'BODY', text: templateForm.body.trim() });
      if (templateForm.footer.trim()) components.push({ type: 'FOOTER', text: templateForm.footer.trim() });
      const buttonLabels = templateForm.buttons.split(',').map((b) => b.trim()).filter(Boolean);
      if (buttonLabels.length > 0) {
        components.push({ type: 'BUTTONS', buttons: buttonLabels.map((text) => ({ type: 'QUICK_REPLY', text })) });
      }
      if (editingTemplate) {
        await api.updateCloudTemplate(token, editingTemplate.name, editingTemplate.language, {
          category: templateForm.category,
          components: components.length ? components : undefined,
        });
        toast.success('Template update submitted');
      } else {
        await api.createCloudTemplate(token, {
          name: templateForm.name.trim(),
          language: templateForm.language.trim(),
          category: templateForm.category,
          components: components.length ? components : undefined,
        });
        toast.success('Template created');
      }
      setTemplateForm(emptyTemplate);
      setEditingTemplate(null);
      setTemplates(await api.listCloudTemplates(token));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not save template');
    } finally { setBusy(null); }
  };

  const deleteTemplate = async () => {
    if (!deleteTarget) return;
    setBusy('delete-template');
    try {
      await api.deleteCloudTemplate(token, deleteTarget.name, deleteTarget.language);
      setDeleteTarget(null);
      setTemplates(await api.listCloudTemplates(token));
      toast.success('Template deleted');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not delete template');
    } finally { setBusy(null); }
  };

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: 'overview', label: 'Overview' },
    { id: 'credentials', label: 'Credentials' },
    { id: 'phone', label: 'Phone' },
    { id: 'templates', label: 'Templates' },
    { id: 'events', label: 'Account events' },
    { id: 'diagnostics', label: 'Diagnostics' },
  ];

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

        <div className="flex flex-wrap gap-1 border-b border-sidebar-border pb-2">
          {tabs.map((item) => (
            <Button
              key={item.id}
              size="sm"
              variant={tab === item.id ? 'default' : 'ghost'}
              onClick={() => setTab(item.id)}
            >
              {item.label}
            </Button>
          ))}
        </div>

        {tab === 'overview' && (
          <div className="space-y-3">
            <div className="grid gap-2 rounded-md bg-sidebar-accent/30 p-3 text-xs md:grid-cols-2">
              <div>Display phone: {diagnostics?.displayPhoneNumber || '—'}</div>
              <div>Verified name: {diagnostics?.verifiedName || '—'}</div>
              <div>Quality: {diagnostics?.qualityRating || '—'}</div>
              <div>Messaging limit: {diagnostics?.messagingLimit || '—'}</div>
              <div>Templates: {diagnostics?.templatesApproved ?? 0} approved / {diagnostics?.templatesPending ?? 0} pending / {diagnostics?.templatesRejected ?? 0} rejected</div>
              <div>Last webhook: {diagnostics?.lastWebhookAt || '—'}</div>
              <div>Effective RPS: {diagnostics?.effectiveRPS ?? '—'}</div>
              <div>App secret: {diagnostics?.appSecretSet ? 'configured' : 'missing'}</div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={verify} disabled={!!busy}>{busy === 'verify' ? 'Verifying…' : 'Verify and subscribe'}</Button>
              <Button variant="outline" onClick={sync} disabled={!!busy}>{busy === 'sync' ? 'Syncing…' : 'Sync templates'}</Button>
            </div>
          </div>
        )}

        {tab === 'credentials' && (
          <div className="space-y-4">
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
            <Button onClick={save} disabled={!!busy}>{busy === 'save' ? 'Saving…' : 'Save credentials'}</Button>
          </div>
        )}

        {tab === 'phone' && (
          <div className="space-y-4">
            <div className="rounded-md border border-sidebar-border p-3">
              <Label>Register phone (six-digit PIN)</Label>
              <div className="mt-2 flex gap-2">
                <Input value={pin} onChange={(e) => setPin(e.target.value)} maxLength={6} placeholder="123456" />
                <Button onClick={register} disabled={!!busy || pin.length !== 6}>
                  {busy === 'register' ? 'Registering…' : 'Register'}
                </Button>
                <Button variant="outline" onClick={deregister} disabled={!!busy}>
                  {busy === 'deregister' ? 'Deregistering…' : 'Deregister'}
                </Button>
              </div>
              <p className="mt-1 text-xs text-sidebar-foreground/60">
                The PIN is sent to Meta and never stored by WaMux.
              </p>
            </div>
            <div className="grid gap-2 rounded-md bg-sidebar-accent/30 p-3 text-xs md:grid-cols-2">
              <div>Status: {diagnostics?.phoneRegistered ? 'registered' : 'not registered'}</div>
              <div>Subscription: {diagnostics?.subscribed ? 'subscribed' : 'not subscribed'}</div>
              <div>Display phone: {diagnostics?.displayPhoneNumber || '—'}</div>
              <div>Quality: {diagnostics?.qualityRating || '—'}</div>
            </div>

            <div className="rounded-md border border-sidebar-border p-3">
              <h3 className="text-sm font-semibold">Embedded Signup</h3>
              <p className="mt-1 text-xs text-sidebar-foreground/60">
                Connect a Meta Business account without copying IDs or tokens.
              </p>
              <div className="mt-2 flex flex-wrap gap-2">
                <Button onClick={startEmbedded} disabled={!!busy}>
                  {busy === 'embedded' ? 'Opening Meta…' : 'Connect with Meta'}
                </Button>
                <Button variant="outline" onClick={() => void loadEmbeddedAssets()} disabled={!embeddedState || !!busy}>
                  {busy === 'embedded-assets' ? 'Loading…' : 'Load assets'}
                </Button>
              </div>
              {embeddedAssets.length > 0 && (
                <div className="mt-2 divide-y divide-sidebar-border rounded border border-sidebar-border">
                  {embeddedAssets.map((asset) => (
                    <div key={`${asset.wabaId}:${asset.phoneNumberId}`} className="flex items-center justify-between gap-2 p-2 text-xs">
                      <div>
                        <div className="font-medium">{asset.wabaName || asset.wabaId}</div>
                        <div className="text-sidebar-foreground/60">{asset.displayPhoneNumber || asset.phoneNumberId}</div>
                      </div>
                      <Button size="sm" onClick={() => void chooseEmbeddedAsset(asset)} disabled={!!busy}>
                        {busy === 'embedded-select' ? 'Connecting…' : 'Use this number'}
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {tab === 'templates' && (
          <div className="space-y-4">
            <div className="rounded-md border border-sidebar-border p-3">
              <div className="mb-2 flex items-center justify-between gap-2">
                <h3 className="text-sm font-semibold">{editingTemplate ? 'Edit template' : 'Create template'}</h3>
                {editingTemplate && (
                  <Button variant="ghost" size="sm" onClick={() => { setEditingTemplate(null); setTemplateForm(emptyTemplate); }}>
                    Cancel edit
                  </Button>
                )}
              </div>
              <div className="grid gap-2 md:grid-cols-3">
                <Input
                  value={templateForm.name}
                  onChange={(e) => setTemplateForm((p) => ({ ...p, name: e.target.value }))}
                  placeholder="order_update"
                  disabled={!!editingTemplate}
                />
                <Input
                  value={templateForm.language}
                  onChange={(e) => setTemplateForm((p) => ({ ...p, language: e.target.value }))}
                  placeholder="en_US"
                  disabled={!!editingTemplate}
                />
                <select
                  className="h-9 rounded-md border border-sidebar-border bg-sidebar px-2 text-sm"
                  value={templateForm.category}
                  onChange={(e) => setTemplateForm((p) => ({ ...p, category: e.target.value }))}
                >
                  <option value="UTILITY">UTILITY</option>
                  <option value="MARKETING">MARKETING</option>
                  <option value="AUTHENTICATION">AUTHENTICATION</option>
                </select>
              </div>
              <div className="mt-2 grid gap-2 md:grid-cols-2">
                <Input
                  value={templateForm.header}
                  onChange={(e) => setTemplateForm((p) => ({ ...p, header: e.target.value }))}
                  placeholder="Header text (optional)"
                />
                <Input
                  value={templateForm.footer}
                  onChange={(e) => setTemplateForm((p) => ({ ...p, footer: e.target.value }))}
                  placeholder="Footer text (optional)"
                />
              </div>
              <textarea
                className="mt-2 min-h-20 w-full rounded-md border border-sidebar-border bg-sidebar p-2 text-sm"
                value={templateForm.body}
                onChange={(e) => setTemplateForm((p) => ({ ...p, body: e.target.value }))}
                placeholder={editingTemplate ? 'Leave blank to keep the current body' : 'Template body text'}
              />
              <Input
                className="mt-2"
                value={templateForm.buttons}
                onChange={(e) => setTemplateForm((p) => ({ ...p, buttons: e.target.value }))}
                placeholder="Quick-reply buttons, comma separated (optional)"
              />
              <div className="mt-2 flex gap-2">
                <Button onClick={saveTemplate} disabled={!!busy}>{busy === 'template' ? 'Saving…' : editingTemplate ? 'Submit update' : 'Create template'}</Button>
                <Button variant="outline" onClick={sync} disabled={!!busy}>{busy === 'sync' ? 'Syncing…' : 'Sync from Meta'}</Button>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-semibold">Templates</h3>
                <Badge variant="secondary">{templates.length}</Badge>
              </div>
              {templates.length === 0 ? (
                <p className="text-xs text-sidebar-foreground/60">No templates synced.</p>
              ) : (
                <div className="max-h-72 divide-y divide-sidebar-border overflow-auto rounded-md border border-sidebar-border">
                  {templates.map((tpl) => (
                    <div key={`${tpl.name}:${tpl.language}`} className="flex items-center justify-between gap-2 p-2 text-xs">
                      <div className="min-w-0">
                        <div className="truncate font-medium">{tpl.name}</div>
                        <div className="truncate text-sidebar-foreground/60">{tpl.language} · {tpl.category || '—'}</div>
                        {tpl.rejectedReason && <div className="truncate text-red-500">{tpl.rejectedReason}</div>}
                      </div>
                      <Badge variant="secondary">{tpl.status || 'UNKNOWN'}</Badge>
                      <div className="flex gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setEditingTemplate(tpl);
                            setTemplateForm({
                              name: tpl.name, language: tpl.language,
                              category: tpl.category || 'UTILITY', header: '', body: '', footer: '', buttons: '',
                            });
                          }}
                        >
                          Edit
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => setDeleteTarget(tpl)}>Delete</Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {tab === 'events' && (
          <div className="space-y-3">
            <div className="flex flex-wrap items-end gap-2">
              <div className="space-y-1">
                <Label>Field filter</Label>
                <Input
                  value={eventField}
                  onChange={(e) => setEventField(e.target.value)}
                  placeholder="message_template_status_update"
                />
              </div>
              <Button variant="outline" onClick={() => void loadEvents()} disabled={!!busy}>
                {busy === 'events' ? 'Loading…' : 'Apply'}
              </Button>
              <Button variant="ghost" onClick={() => { setEventField(''); void loadEvents(''); }}>Clear</Button>
            </div>
            {events.length === 0 ? (
              <p className="text-xs text-sidebar-foreground/60">No account events recorded.</p>
            ) : (
              <div className="max-h-80 divide-y divide-sidebar-border overflow-auto rounded-md border border-sidebar-border">
                {events.map((event) => (
                  <div key={event.id} className="space-y-1 p-2 text-xs">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="font-medium">{event.field}</span>
                      <Badge variant="secondary">{event.eventType || 'event'}</Badge>
                    </div>
                    <div className="text-sidebar-foreground/60">{event.receivedAt || '—'}</div>
                    {event.payload && (
                      <pre className="max-h-24 overflow-auto rounded bg-sidebar-accent/40 p-2 text-[11px]">
                        {event.payload}
                      </pre>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {tab === 'diagnostics' && (
          <div className="space-y-2 text-xs">
            <div className="grid gap-2 rounded-md bg-sidebar-accent/30 p-3 md:grid-cols-2">
              <div>Token valid: {diagnostics?.tokenValid ? 'yes' : 'no'}</div>
              <div>App secret set: {diagnostics?.appSecretSet ? 'yes' : 'no'}</div>
              <div>Phone registered: {diagnostics?.phoneRegistered ? 'yes' : 'no'}</div>
              <div>App subscribed: {diagnostics?.subscribed ? 'yes' : 'no'}</div>
              <div>Quality: {diagnostics?.qualityRating || '—'}</div>
              <div>Messaging limit: {diagnostics?.messagingLimit || '—'}</div>
              <div>Effective RPS: {diagnostics?.effectiveRPS ?? '—'}</div>
              <div>Last webhook: {diagnostics?.lastWebhookAt || '—'}</div>
            </div>
            {diagnostics?.lastGraphError && (
              <div className="rounded-md border border-red-500/30 bg-red-500/5 p-3 text-red-500">
                {diagnostics.lastGraphError}
              </div>
            )}
            <pre className="max-h-72 overflow-auto rounded-md border border-sidebar-border p-3">
              {JSON.stringify(diagnostics, null, 2)}
            </pre>
          </div>
        )}
      </CardContent>

      <ConfirmDialog
        open={!!deleteTarget}
        title="Delete template?"
        description={deleteTarget ? `${deleteTarget.name} (${deleteTarget.language}) will be deleted from Meta.` : ''}
        confirmLabel={busy === 'delete-template' ? 'Deleting…' : 'Delete'}
        cancelLabel="Cancel"
        destructive
        onConfirm={() => void deleteTemplate()}
        onCancel={() => setDeleteTarget(null)}
      />
    </Card>
  );
}
