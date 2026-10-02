import LegalLayout, { LegalSection } from '@/components/base/LegalLayout';
import { PRODUCT_NAME, REPO } from '@/constants/branding';
import { useI18n } from '@/i18n/I18nContext';

/**
 * Termos de Serviço.
 *
 * Public page linked from the login screen. Describes what this self-hosted
 * WhatsApp instance manager actually does and the rules for using it.
 */
export default function Terms() {
  const { t } = useI18n();
  return (
    <LegalLayout
      title={t('legal.terms.title')}
      subtitle={t('legal.terms.subtitle', { product: PRODUCT_NAME })}
      updatedAt={t('legal.updatedDate')}
    >
      <LegalSection title={t('legal.terms.s1.title')}>
        <p>{t('legal.terms.s1.p1', { product: PRODUCT_NAME })}</p>
        <p>{t('legal.terms.s1.p2')}</p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s2.title')}>
        <p>{t('legal.terms.s2.p1')}</p>
        <p>{t('legal.terms.s2.p2')}</p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s3.title')}>
        <ul className="list-disc space-y-1 pl-5">
          <li>{t('legal.terms.s3.li1')}</li>
          <li>{t('legal.terms.s3.li2')}</li>
          <li>{t('legal.terms.s3.li3')}</li>
        </ul>
        <p>{t('legal.terms.s3.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s4.title')}>
        <p>
          {t('legal.terms.s4.p1Pre')}{' '}
          <code className="text-foreground">GLOBAL_API_KEY</code>{' '}
          {t('legal.terms.s4.p1Post')}
        </p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s5.title')}>
        <p>{t('legal.terms.s5.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s6.title')}>
        <p>{t('legal.terms.s6.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s7.title')}>
        <p>{t('legal.terms.s7.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s8.title')}>
        <p>
          {t('legal.terms.s8.p1Pre')}{' '}
          <code className="text-foreground">LICENSE</code>
          {t('legal.terms.s8.p1Between1')}{' '}
          <code className="text-foreground">NOTICE</code>{' '}
          {t('legal.terms.s8.p1Between2')}{' '}
          <code className="text-foreground">TRADEMARKS.md</code>{' '}
          {t('legal.terms.s8.p1Post')}
        </p>
        <div className="flex flex-wrap gap-4 pt-1">
          <a
            className="text-primary hover:underline"
            href={REPO}
            target="_blank"
            rel="noreferrer noopener"
          >
            {t('legal.terms.s8.repo')}
          </a>
        </div>
      </LegalSection>

      <LegalSection title={t('legal.terms.s9.title')}>
        <p>{t('legal.terms.s9.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.terms.s10.title')}>
        <p>{t('legal.terms.s10.p1')}</p>
      </LegalSection>
    </LegalLayout>
  );
}
