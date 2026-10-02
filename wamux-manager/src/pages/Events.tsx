import { useI18n } from '@/i18n/I18nContext';

function Events() {
  const { t } = useI18n();
  return (
    <div className="p-4 sm:p-6">
      <h1 className="mb-4 text-2xl font-bold text-foreground">
        {t('dashboard.eventsTitle')}
      </h1>
      <p className="text-muted-foreground">{t('dashboard.eventsPlaceholder')}</p>
    </div>
  );
}

export default Events;
