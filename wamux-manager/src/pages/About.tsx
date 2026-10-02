import { Info, ShieldCheck, Scale } from 'lucide-react';
import GithubIcon from '@/components/base/GithubIcon';
import { COPYRIGHT_LINE, PRODUCT_NAME, REPO } from '@/constants/branding';
import { useI18n } from '@/i18n/I18nContext';

/**
 * About / "Sobre".
 *
 * Also serves as the administrator-visible "this system uses WaMux" notice.
 */
export default function About() {
  const { t } = useI18n();

  return (
    <div className="h-full overflow-y-auto p-4 sm:p-6">
      <div className="mx-auto max-w-3xl space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-foreground">{t('about.title')}</h1>
          <p className="text-sm text-muted-foreground">
            {PRODUCT_NAME} — {t('home.tagline')}
          </p>
        </div>

        {/* Administrator-visible notice that this system uses WaMux. */}
        <div className="rounded-xl border border-primary/40 bg-primary/5 p-5">
          <div className="flex items-start gap-3">
            <ShieldCheck className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
            <div className="space-y-1">
              <h2 className="font-semibold text-foreground">
                {t('about.noticeTitle', { product: PRODUCT_NAME })}
              </h2>
              <p className="text-sm text-muted-foreground">
                {t('about.noticeBody', { product: PRODUCT_NAME })}
              </p>
            </div>
          </div>
        </div>

        {/* License. */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-5">
          <div className="flex items-start gap-3">
            <Scale className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
            <div className="space-y-2">
              <h2 className="font-semibold text-foreground">
                {t('about.licenseTitle')}
              </h2>
              <p className="text-sm text-muted-foreground">
                {t('about.licenseBody')}
              </p>
              <p className="text-sm text-muted-foreground">{COPYRIGHT_LINE}</p>
            </div>
          </div>
        </div>

        {/* About the project. */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-5">
          <div className="flex items-start gap-3">
            <Info className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
            <div className="space-y-2">
              <h2 className="font-semibold text-foreground">
                {t('about.projectTitle', { product: PRODUCT_NAME })}
              </h2>
              <p className="text-sm text-muted-foreground">
                {t('about.projectBody')}
              </p>
              <a
                className="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
                href={REPO}
                target="_blank"
                rel="noreferrer noopener"
              >
                <GithubIcon className="h-4 w-4" />
                {t('about.repo')}
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
