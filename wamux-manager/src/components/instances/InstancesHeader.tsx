/**
 * Instances Header Component
 * Header for instances page with search and actions
 */

import { Plus } from 'lucide-react';
import BaseHeader, { type HeaderAction } from '../base/BaseHeader';
import { useI18n } from '@/i18n/I18nContext';

interface InstancesHeaderProps {
  totalCount: number;
  selectedCount: number;
  searchValue: string;
  onSearchChange: (value: string) => void;
  onNewInstance: () => void;
  onRefresh: () => void;
  isRefreshing?: boolean;
  onClearSelection: () => void;
}

export default function InstancesHeader({
  totalCount,
  selectedCount,
  searchValue,
  onSearchChange,
  onNewInstance,
  onRefresh,
  isRefreshing = false,
  onClearSelection,
}: InstancesHeaderProps) {
  const { t } = useI18n();
  const primaryAction: HeaderAction = {
    label: t('instances.newInstance'),
    icon: <Plus className="h-4 w-4" />,
    onClick: onNewInstance,
  };

  return (
    <BaseHeader
      title={t('instances.title')}
      subtitle={t('instances.subtitle')}
      totalCount={totalCount}
      selectedCount={selectedCount}
      searchValue={searchValue}
      onSearchChange={onSearchChange}
      searchPlaceholder={t('instances.searchPlaceholder')}
      primaryAction={primaryAction}
      refreshAction={{
        onClick: onRefresh,
        isRefreshing,
        title: t('instances.refreshTitle'),
      }}
      onClearSelection={onClearSelection}
      showFilters={false}
      className="mb-4"
    />
  );
}
