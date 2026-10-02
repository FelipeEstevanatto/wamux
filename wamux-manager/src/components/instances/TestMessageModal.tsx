import { useMemo, useState } from 'react';
import { FlaskConical, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui';
import * as instancesApi from '@/services/api/instances';
import type { Instance } from '@/types/instance';
import { useI18n } from '@/i18n/I18nContext';

interface TestMessageModalProps {
  open: boolean;
  onClose: () => void;
  instance: Instance | null;
}

type TestScenarioId =
  | 'btn_reply_1'
  | 'btn_reply_3'
  | 'btn_copy'
  | 'btn_url'
  | 'btn_call'
  | 'btn_pix'
  | 'btn_cta_group'
  | 'list'
  | 'carousel_reply'
  | 'carousel_url'
  | 'carousel_call'
  | 'carousel_copy';

type TestScenario = {
  id: TestScenarioId;
  group: 'button' | 'list' | 'carousel';
  label: string;
  description: string;
  endpoint: 'button' | 'list' | 'carousel';
};

const SCENARIOS: TestScenario[] = [
  {
    id: 'btn_reply_1',
    group: 'button',
    endpoint: 'button',
    label: 'modals.scenario.btnReply1.label',
    description: 'modals.scenario.btnReply1.desc',
  },
  {
    id: 'btn_reply_3',
    group: 'button',
    endpoint: 'button',
    label: 'modals.scenario.btnReply3.label',
    description: 'modals.scenario.btnReply3.desc',
  },
  {
    id: 'btn_copy',
    group: 'button',
    endpoint: 'button',
    label: 'modals.scenario.btnCopy.label',
    description: 'modals.scenario.btnCopy.desc',
  },
  {
    id: 'btn_url',
    group: 'button',
    endpoint: 'button',
    label: 'modals.scenario.btnUrl.label',
    description: 'modals.scenario.btnUrl.desc',
  },
  {
    id: 'btn_call',
    group: 'button',
    endpoint: 'button',
    label: 'modals.scenario.btnCall.label',
    description: 'modals.scenario.btnCall.desc',
  },
  {
    id: 'btn_pix',
    group: 'button',
    endpoint: 'button',
    label: 'modals.scenario.btnPix.label',
    description: 'modals.scenario.btnPix.desc',
  },
  {
    id: 'btn_cta_group',
    group: 'button',
    endpoint: 'button',
    label: 'modals.scenario.btnCtaGroup.label',
    description: 'modals.scenario.btnCtaGroup.desc',
  },
  {
    id: 'list',
    group: 'list',
    endpoint: 'list',
    label: 'modals.scenario.list.label',
    description: 'modals.scenario.list.desc',
  },
  {
    id: 'carousel_reply',
    group: 'carousel',
    endpoint: 'carousel',
    label: 'modals.scenario.carouselReply.label',
    description: 'modals.scenario.carouselReply.desc',
  },
  {
    id: 'carousel_url',
    group: 'carousel',
    endpoint: 'carousel',
    label: 'modals.scenario.carouselUrl.label',
    description: 'modals.scenario.carouselUrl.desc',
  },
  {
    id: 'carousel_call',
    group: 'carousel',
    endpoint: 'carousel',
    label: 'modals.scenario.carouselCall.label',
    description: 'modals.scenario.carouselCall.desc',
  },
  {
    id: 'carousel_copy',
    group: 'carousel',
    endpoint: 'carousel',
    label: 'modals.scenario.carouselCopy.label',
    description: 'modals.scenario.carouselCopy.desc',
  },
];

const GROUP_LABELS: Record<TestScenario['group'], string> = {
  button: 'modals.group.button',
  list: 'modals.group.list',
  carousel: 'modals.group.carousel',
};

function buildPayload(
  scenarioId: TestScenarioId,
  number: string,
  t: (key: string, vars?: Record<string, string | number>) => string,
): Record<string, unknown> {
  switch (scenarioId) {
    case 'btn_reply_1':
      return {
        number,
        title: t('modals.payload.reply1.title'),
        description: t('modals.payload.reply1.desc'),
        footer: 'WaMux',
        buttons: [
          { type: 'reply', displayText: t('modals.payload.confirm'), id: 'test_reply_1' },
        ],
      };
    case 'btn_reply_3':
      return {
        number,
        title: t('modals.payload.reply3.title'),
        description: t('modals.payload.reply3.desc'),
        footer: 'WaMux',
        buttons: [
          { type: 'reply', displayText: t('modals.payload.optionA'), id: 'test_a' },
          { type: 'reply', displayText: t('modals.payload.optionB'), id: 'test_b' },
          { type: 'reply', displayText: t('modals.payload.optionC'), id: 'test_c' },
        ],
      };
    case 'btn_copy':
      return {
        number,
        title: t('modals.payload.copy.title'),
        description: t('modals.payload.copy.desc'),
        footer: 'WaMux',
        buttons: [
          {
            type: 'copy',
            displayText: t('modals.payload.copyCoupon'),
            copyCode: 'PROMO2026',
          },
        ],
      };
    case 'btn_url':
      return {
        number,
        title: t('modals.payload.url.title'),
        description: t('modals.payload.url.desc'),
        footer: 'WaMux',
        buttons: [
          {
            type: 'url',
            displayText: t('modals.payload.openSite'),
            url: 'https://wamuxapi.com',
          },
        ],
      };
    case 'btn_call':
      return {
        number,
        title: t('modals.payload.call.title'),
        description: t('modals.payload.call.desc'),
        footer: 'WaMux',
        buttons: [
          {
            type: 'call',
            displayText: t('modals.payload.callNow'),
            phoneNumber: '+' + number.replace(/\D/g, ''),
          },
        ],
      };
    case 'btn_pix':
      return {
        number,
        title: t('modals.payload.pix.title'),
        description: t('modals.payload.pix.desc'),
        footer: 'WaMux',
        buttons: [
          {
            type: 'pix',
            currency: 'BRL',
            name: t('modals.payload.pix.storeName'),
            keyType: 'cpf',
            key: '12345678900',
          },
        ],
      };
    case 'btn_cta_group':
      return {
        number,
        title: t('modals.payload.ctaGroup.title'),
        description: t('modals.payload.ctaGroup.desc'),
        footer: 'WaMux',
        buttons: [
          {
            type: 'copy',
            displayText: t('modals.payload.copyCoupon'),
            copyCode: 'CTA2026',
          },
          {
            type: 'url',
            displayText: t('modals.payload.openSite'),
            url: 'https://wamuxapi.com',
          },
          {
            type: 'call',
            displayText: t('modals.payload.callNow'),
            phoneNumber: '+' + number.replace(/\D/g, ''),
          },
        ],
      };
    case 'list':
      return {
        number,
        title: t('modals.payload.list.title'),
        description: t('modals.payload.list.desc'),
        buttonText: t('modals.payload.list.buttonText'),
        footerText: 'WaMux',
        sections: [
          {
            title: t('modals.payload.list.plans'),
            rows: [
              {
                title: t('modals.payload.list.basicPlan'),
                description: t('modals.payload.list.basicPrice'),
                rowId: 'plan_basic',
              },
              {
                title: t('modals.payload.list.proPlan'),
                description: t('modals.payload.list.proPrice'),
                rowId: 'plan_pro',
              },
            ],
          },
          {
            title: t('modals.payload.list.support'),
            rows: [
              {
                title: t('modals.payload.list.talkToAgent'),
                description: t('modals.payload.businessHours'),
                rowId: 'support_agent',
              },
              {
                title: t('modals.payload.list.helpCenter'),
                description: t('modals.payload.list.articlesFaq'),
                rowId: 'support_kb',
              },
            ],
          },
        ],
      };
    case 'carousel_reply':
      return {
        number,
        body: t('modals.payload.carouselReply.body'),
        footer: 'WaMux',
        cards: [
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/replyA/600/400',
            },
            body: { text: t('modals.payload.carouselReply.cardABasic') },
            footer: t('modals.payload.carouselReply.priceBasic'),
            buttons: [
              { type: 'REPLY', displayText: t('modals.payload.carouselReply.subscribeBasic'), id: 'reply_basic' },
              { type: 'REPLY', displayText: t('modals.payload.learnMore'),     id: 'reply_basic_info' },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/replyB/600/400',
            },
            body: { text: t('modals.payload.carouselReply.cardBPro') },
            footer: t('modals.payload.carouselReply.pricePro'),
            buttons: [
              { type: 'REPLY', displayText: t('modals.payload.carouselReply.subscribePro'), id: 'reply_pro' },
              { type: 'REPLY', displayText: t('modals.payload.learnMore'),  id: 'reply_pro_info' },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/replyC/600/400',
            },
            body: { text: t('modals.payload.carouselReply.cardCBusiness') },
            footer: t('modals.payload.carouselReply.priceBusiness'),
            buttons: [
              { type: 'REPLY', displayText: t('modals.payload.carouselReply.subscribeBusiness'), id: 'reply_business' },
              { type: 'REPLY', displayText: t('modals.payload.learnMore'),        id: 'reply_business_info' },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/replyD/600/400',
            },
            body: { text: t('modals.payload.carouselReply.cardDEnterprise') },
            footer: t('modals.payload.carouselReply.onRequest'),
            buttons: [
              { type: 'REPLY', displayText: t('modals.payload.carouselReply.talkToSales'), id: 'reply_enterprise' },
            ],
          },
        ],
      };
    case 'carousel_url':
      return {
        number,
        body: t('modals.payload.carouselUrl.body'),
        footer: 'WaMux',
        cards: [
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/urlA/600/400',
            },
            body: { text: t('modals.payload.carouselUrl.cardASite') },
            footer: t('modals.payload.carouselUrl.opensMainSite'),
            buttons: [
              {
                type: 'URL',
                displayText: t('modals.payload.openSite'),
                id: 'https://wamuxapi.com',
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/urlB/600/400',
            },
            body: { text: t('modals.payload.carouselUrl.cardBDocs') },
            footer: t('modals.payload.carouselUrl.opensApiDocs'),
            buttons: [
              {
                type: 'URL',
                displayText: t('modals.payload.carouselUrl.viewDocs'),
                id: 'https://doc.wamuxapi.com',
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/urlC/600/400',
            },
            body: { text: t('modals.payload.carouselUrl.cardCGithub') },
            footer: t('modals.payload.carouselUrl.opensRepo'),
            buttons: [
              {
                type: 'URL',
                displayText: t('modals.payload.carouselUrl.openGithub'),
                id: 'https://github.com/FelipeEstevanatto/wamux',
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/urlD/600/400',
            },
            body: { text: t('modals.payload.carouselUrl.cardDCommunity') },
            footer: t('modals.payload.carouselUrl.joinCommunity'),
            buttons: [
              {
                type: 'URL',
                displayText: t('modals.payload.carouselUrl.enterCommunity'),
                id: 'https://wamuxapi.com/community',
              },
            ],
          },
        ],
      };
    case 'carousel_call':
      return {
        number,
        body: t('modals.payload.carouselCall.body'),
        footer: 'WaMux',
        cards: [
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/callA/600/400',
            },
            body: { text: t('modals.payload.carouselCall.cardAGeneral') },
            footer: t('modals.payload.businessHours'),
            buttons: [
              {
                type: 'CALL',
                displayText: t('modals.payload.carouselCall.callGeneral'),
                id: '+' + number.replace(/\D/g, ''),
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/callB/600/400',
            },
            body: { text: t('modals.payload.carouselCall.cardBTechSupport') },
            footer: '24x7',
            buttons: [
              {
                type: 'CALL',
                displayText: t('modals.payload.carouselCall.callSupport'),
                id: '+' + number.replace(/\D/g, ''),
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/callC/600/400',
            },
            body: { text: t('modals.payload.carouselCall.cardCFinance') },
            footer: t('modals.payload.carouselCall.weekdaysHours'),
            buttons: [
              {
                type: 'CALL',
                displayText: t('modals.payload.carouselCall.callFinance'),
                id: '+' + number.replace(/\D/g, ''),
              },
            ],
          },
        ],
      };
    case 'carousel_copy':
      return {
        number,
        body: t('modals.payload.carouselCopy.body'),
        footer: 'WaMux',
        cards: [
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/copyA/600/400',
            },
            body: { text: t('modals.payload.carouselCopy.cardAFirstCoupon') },
            footer: t('modals.payload.carouselCopy.discount10'),
            buttons: [
              {
                type: 'COPY',
                displayText: t('modals.payload.copyCoupon'),
                copyCode: 'BEMVINDO10',
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/copyB/600/400',
            },
            body: { text: t('modals.payload.carouselCopy.cardBBlackFriday') },
            footer: t('modals.payload.carouselCopy.discount30'),
            buttons: [
              {
                type: 'COPY',
                displayText: t('modals.payload.copyCoupon'),
                copyCode: 'BLACK30',
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/copyC/600/400',
            },
            body: { text: t('modals.payload.carouselCopy.cardCAnnual') },
            footer: t('modals.payload.carouselCopy.freeTwoMonths'),
            buttons: [
              {
                type: 'COPY',
                displayText: t('modals.payload.copyCoupon'),
                copyCode: 'ANUAL2MESES',
              },
            ],
          },
          {
            header: {
              imageUrl: 'https://picsum.photos/seed/copyD/600/400',
            },
            body: { text: t('modals.payload.carouselCopy.cardDVip') },
            footer: t('modals.payload.carouselCopy.exclusiveClients'),
            buttons: [
              {
                type: 'COPY',
                displayText: t('modals.payload.carouselCopy.copyVipCoupon'),
                copyCode: 'VIP2026',
              },
            ],
          },
        ],
      };
  }
}

function TestMessageModal({ open, onClose, instance }: TestMessageModalProps) {
  const { t } = useI18n();
  const [number, setNumber] = useState('');
  const [scenarioId, setScenarioId] =
    useState<TestScenarioId>('btn_reply_1');
  const [isSending, setIsSending] = useState(false);
  const [result, setResult] = useState<
    | { ok: true; messageId: string }
    | { ok: false; error: string }
    | null
  >(null);

  const scenario = useMemo(
    () => SCENARIOS.find((s) => s.id === scenarioId)!,
    [scenarioId],
  );

  const groupedScenarios = useMemo(() => {
    const groups: Record<TestScenario['group'], TestScenario[]> = {
      button: [],
      list: [],
      carousel: [],
    };
    for (const s of SCENARIOS) groups[s.group].push(s);
    return groups;
  }, []);

  const handleClose = () => {
    if (isSending) return;
    setResult(null);
    setNumber('');
    setScenarioId('btn_reply_1');
    onClose();
  };

  const handleSend = async () => {
    if (!instance?.apikey) {
      toast.error(t('modals.testTokenMissing'));
      return;
    }

    const digits = number.replace(/\D/g, '');
    if (digits.length < 10) {
      toast.error(t('modals.invalidNumber'));
      return;
    }

    setIsSending(true);
    setResult(null);

    try {
      const payload = buildPayload(scenarioId, digits, t);
      let response;
      if (scenario.endpoint === 'button') {
        response = await instancesApi.sendButtonMessage(
          instance.apikey,
          payload,
        );
      } else if (scenario.endpoint === 'list') {
        response = await instancesApi.sendListMessage(
          instance.apikey,
          payload,
        );
      } else {
        response = await instancesApi.sendCarouselMessage(
          instance.apikey,
          payload,
        );
      }

      const messageId =
        (response.data as { Info?: { ID?: string } } | null)?.Info?.ID ||
        t('modals.noMessageId');
      setResult({ ok: true, messageId });
      toast.success(t('modals.testSentSuccess'), {
        description: t(scenario.label),
      });
    } catch (err: unknown) {
      const axiosErr = err as {
        response?: { data?: { error?: string }; status?: number };
        message?: string;
      };
      const msg =
        axiosErr?.response?.data?.error ||
        axiosErr?.message ||
        t('modals.unknownSendError');
      setResult({ ok: false, error: msg });
      toast.error(t('modals.testSendFailed'), { description: msg });
    } finally {
      setIsSending(false);
    }
  };

  if (!open || !instance) return null;

  return (
    <Dialog
      open={open && !!instance}
      onOpenChange={(next) => {
        if (!next) handleClose();
      }}
    >
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FlaskConical className="h-5 w-5 text-purple-500" />
            {t('modals.testMessagesTitle', { name: instance.instanceName })}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <label
              htmlFor="test-number"
              className="mb-1 block text-sm font-medium text-foreground"
            >
              {t('modals.targetNumber')}
            </label>
            <input
              id="test-number"
              type="text"
              placeholder="5582988898565"
              disabled={isSending}
              value={number}
              onChange={(e) => setNumber(e.target.value)}
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            />
            <p className="mt-1 text-xs text-muted-foreground">
              {t('modals.targetNumberHint')}
            </p>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-foreground">
              {t('modals.testMode')}
            </label>
            <div className="space-y-3 rounded-md border border-input bg-background/50 p-3">
              {(Object.keys(groupedScenarios) as TestScenario['group'][]).map(
                (group) => (
                  <div key={group}>
                    <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      {t(GROUP_LABELS[group])}
                    </p>
                    <div className="grid grid-cols-1 gap-1 sm:grid-cols-2">
                      {groupedScenarios[group].map((s) => (
                        <label
                          key={s.id}
                          className={`flex cursor-pointer items-start gap-2 rounded-md border p-2 text-sm transition-colors ${
                            scenarioId === s.id
                              ? 'border-primary bg-primary/10'
                              : 'border-transparent hover:bg-accent'
                          }`}
                        >
                          <input
                            type="radio"
                            name="scenario"
                            value={s.id}
                            checked={scenarioId === s.id}
                            disabled={isSending}
                            onChange={() => setScenarioId(s.id)}
                            className="mt-1"
                          />
                          <span className="flex-1">
                            <span className="block font-medium text-foreground">
                              {t(s.label)}
                            </span>
                            <span className="block text-xs text-muted-foreground">
                              {t(s.description)}
                            </span>
                          </span>
                        </label>
                      ))}
                    </div>
                  </div>
                ),
              )}
            </div>
          </div>

          {result && (
            <div
              className={`rounded-md border p-3 text-sm ${
                result.ok
                  ? 'border-green-500/40 bg-green-500/10 text-green-400'
                  : 'border-destructive/40 bg-destructive/10 text-destructive'
              }`}
            >
              {result.ok ? (
                <>
                  <p className="font-medium">{t('modals.sentSuccess')}</p>
                  <p className="font-mono text-xs">
                    {t('modals.messageId')} {result.messageId}
                  </p>
                </>
              ) : (
                <>
                  <p className="font-medium">{t('modals.sendFailed')}</p>
                  <p className="text-xs">{result.error}</p>
                </>
              )}
            </div>
          )}

          <div className="flex gap-2 pt-2">
            <button
              type="button"
              onClick={handleClose}
              disabled={isSending}
              className="flex-1 rounded-md border border-input px-4 py-2 text-sm font-medium text-foreground hover:bg-accent disabled:opacity-50"
            >
              {t('common.close')}
            </button>
            <button
              type="button"
              onClick={handleSend}
              disabled={isSending || !number.trim()}
              className="flex-1 inline-flex items-center justify-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {isSending ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  {t('modals.sending')}
                </>
              ) : (
                <>
                  <FlaskConical className="h-4 w-4" />
                  {t('modals.sendTest')}
                </>
              )}
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export default TestMessageModal;
