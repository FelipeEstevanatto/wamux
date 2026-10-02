import LegalLayout, { LegalSection } from '@/components/base/LegalLayout';
import { PRODUCT_NAME } from '@/constants/branding';
import { useI18n } from '@/i18n/I18nContext';

/**
 * Política de Privacidade.
 *
 * Public page linked from the login screen. Explains, in plain terms, which
 * data this self-hosted manager handles, where it is stored and who is
 * responsible for it (the operator, not the maintainers).
 */
export default function Privacy() {
  const { t } = useI18n();
  return (
    <LegalLayout
      title={t('legal.privacy.title')}
      subtitle={t('legal.privacy.subtitle', { product: PRODUCT_NAME })}
      updatedAt={t('legal.updatedDate')}
    >
      <LegalSection title={t('legal.privacy.s1.title')}>
        <p>{t('legal.privacy.s1.p1', { product: PRODUCT_NAME })}</p>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s2.title')}>
        <ul className="list-disc space-y-1 pl-5">
          <li>
            <span className="text-foreground">
              {t('legal.privacy.s2.li1Label')}
            </span>{' '}
            {t('legal.privacy.s2.li1Pre')}{' '}
            <code className="text-foreground">GLOBAL_API_KEY</code>{' '}
            {t('legal.privacy.s2.li1Post')}
          </li>
          <li>
            <span className="text-foreground">
              {t('legal.privacy.s2.li2Label')}
            </span>{' '}
            {t('legal.privacy.s2.li2Text')}
          </li>
          <li>
            <span className="text-foreground">
              {t('legal.privacy.s2.li3Label')}
            </span>{' '}
            {t('legal.privacy.s2.li3Pre')}
            <code className="text-foreground">DATABASE_SAVE_MESSAGES</code>
            {t('legal.privacy.s2.li3Post')}
          </li>
        </ul>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s3.title')}>
        <p>{t('legal.privacy.s3.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s4.title')}>
        <p>{t('legal.privacy.s4.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s5.title')}>
        <p>{t('legal.privacy.s5.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s6.title')}>
        <p>
          {t('legal.privacy.s6.p1Pre')}{' '}
          <code className="text-foreground">GLOBAL_API_KEY</code>{' '}
          {t('legal.privacy.s6.p1Mid')}
          <code className="text-foreground">SWAGGER_ENABLED=false</code>
          {t('legal.privacy.s6.p1Post')}
        </p>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s7.title')}>
        <p>
          {t('legal.privacy.s7.p1Pre')}{' '}
          <code className="text-foreground">MESSAGE_RETENTION_DAYS</code>{' '}
          {t('legal.privacy.s7.p1Mid')}
          <code className="text-foreground">0</code>{' '}
          {t('legal.privacy.s7.p1Post')}
        </p>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s8.title')}>
        <p>{t('legal.privacy.s8.p1')}</p>
      </LegalSection>

      <LegalSection title={t('legal.privacy.s9.title')}>
        <p>{t('legal.privacy.s9.p1')}</p>
      </LegalSection>
    </LegalLayout>
  );
}
