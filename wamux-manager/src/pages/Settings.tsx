import { useI18n } from '@/i18n/I18nContext';

function Settings() {
  const { t } = useI18n();
  return (
    <div className="p-4 sm:p-6">
      <h1 className="mb-4 text-2xl font-bold text-gray-900">
        {t('dashboard.settingsTitle')}
      </h1>
      <p className="text-gray-600">{t('dashboard.settingsPlaceholder')}</p>
    </div>
  );
}

export default Settings;
