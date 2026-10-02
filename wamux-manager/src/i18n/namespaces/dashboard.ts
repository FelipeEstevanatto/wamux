/**
 * Translation namespace for the Dashboard / Settings / Events / BaseHeader group.
 * Keys must be prefixed (e.g. `dashboard.*`) to avoid clashing with other namespaces.
 */
export const dashboardNs: {
  pt: Record<string, string>;
  en: Record<string, string>;
} = {
  pt: {
    'dashboard.subtitle': 'Visão geral do sistema e das instâncias',
    'dashboard.deviceNameTitle':
      'Nome exibido como aparelho conectado no WhatsApp (OS_NAME)',
    'dashboard.kpiConnected': 'Conectadas',
    'dashboard.connectedCount': '{count} conectada(s)',
    'dashboard.percentOfTotal': '{pct}% do total',
    'dashboard.savedInDb': 'salvas no banco',
    'dashboard.kpiContacts': 'Contatos',
    'dashboard.summingConnected': 'somando instâncias conectadas',
    'dashboard.kpiHostRam': 'RAM do host',
    'dashboard.hostUnavailable': 'host indisponível',
    'dashboard.kpiLoad': 'Carga (1m)',
    'dashboard.loadUnavailable': 'load indisponível',
    'dashboard.kpiGoroutines': 'Goroutines',
    'dashboard.heapValue': 'heap {size}',
    'dashboard.kpiUptime': 'Uptime',
    'dashboard.loadTitle':
      'Carga média (1 min): média de tarefas executáveis (rodando ou na fila). {load1}',
    'dashboard.loadTitleCapacity':
      ' em {cpus} CPUs ≈ {loadPct}% da capacidade',
    'dashboard.loadTitle5m': ' · 5m {load5}',
    'dashboard.loadTitle15m': ' · 15m {load15}',
    'dashboard.storageTitle': 'Armazenamento',
    'dashboard.disk': 'Disco',
    'dashboard.storageData': 'Dados',
    'dashboard.dataFilesValue': '{size} · {count} arquivo(s)',
    'dashboard.storageDatabase': 'Banco de dados',
    'dashboard.postgresql': 'PostgreSQL',
    'dashboard.dbMessagesLabel': 'messages',
    'dashboard.storageMedia': 'Mídia',
    'dashboard.mediaNotConfigured': 'não configurada',
    'dashboard.mediaFilesHint': 'arquivos enviados/recebidos',
    'dashboard.mediaDisabledHint': 'mídia não é gravada localmente',
    'dashboard.messagesPerDay': 'Mensagens por dia',
    'dashboard.noMessagesData':
      'Sem dados. Ative DATABASE_SAVE_MESSAGES para registrar mensagens.',
    'dashboard.fullDashboard': 'Dashboard completo',
    'dashboard.fullDashboardDesc':
      'Gráficos, conversas mais ativas, instâncias e logs.',
    'dashboard.openFullDashboard': 'Abrir dashboard completo',

    'dashboard.settingsTitle': 'Configurações',
    'dashboard.settingsPlaceholder':
      'As configurações serão implementadas aqui...',

    'dashboard.eventsTitle': 'Eventos',
    'dashboard.eventsPlaceholder':
      'O monitor de eventos será implementado aqui...',

    'dashboard.searchPlaceholder': 'Buscar...',
    'dashboard.refresh': 'Atualizar',
    'dashboard.filters': 'Filtros',
    'dashboard.moreActions': 'Mais ações',
    'dashboard.selectedSingular': 'selecionado',
    'dashboard.selectedPlural': 'selecionados',
    'dashboard.clear': 'Limpar',
    'dashboard.removeFilter': 'Remover filtro {label}',
  },
  en: {
    'dashboard.subtitle': 'Overview of the system and instances',
    'dashboard.deviceNameTitle':
      'Name shown as the connected device in WhatsApp (OS_NAME)',
    'dashboard.kpiConnected': 'Connected',
    'dashboard.connectedCount': '{count} connected',
    'dashboard.percentOfTotal': '{pct}% of total',
    'dashboard.savedInDb': 'saved in the database',
    'dashboard.kpiContacts': 'Contacts',
    'dashboard.summingConnected': 'summing connected instances',
    'dashboard.kpiHostRam': 'Host RAM',
    'dashboard.hostUnavailable': 'host unavailable',
    'dashboard.kpiLoad': 'Load (1m)',
    'dashboard.loadUnavailable': 'load unavailable',
    'dashboard.kpiGoroutines': 'Goroutines',
    'dashboard.heapValue': 'heap {size}',
    'dashboard.kpiUptime': 'Uptime',
    'dashboard.loadTitle':
      'Load average (1 min): average number of runnable tasks (running or queued). {load1}',
    'dashboard.loadTitleCapacity': ' across {cpus} CPUs ≈ {loadPct}% of capacity',
    'dashboard.loadTitle5m': ' · 5m {load5}',
    'dashboard.loadTitle15m': ' · 15m {load15}',
    'dashboard.storageTitle': 'Storage',
    'dashboard.disk': 'Disk',
    'dashboard.storageData': 'Data',
    'dashboard.dataFilesValue': '{size} · {count} file(s)',
    'dashboard.storageDatabase': 'Database',
    'dashboard.postgresql': 'PostgreSQL',
    'dashboard.dbMessagesLabel': 'messages',
    'dashboard.storageMedia': 'Media',
    'dashboard.mediaNotConfigured': 'not configured',
    'dashboard.mediaFilesHint': 'files sent/received',
    'dashboard.mediaDisabledHint': 'media is not stored locally',
    'dashboard.messagesPerDay': 'Messages per day',
    'dashboard.noMessagesData':
      'No data. Enable DATABASE_SAVE_MESSAGES to record messages.',
    'dashboard.fullDashboard': 'Full dashboard',
    'dashboard.fullDashboardDesc':
      'Charts, most active conversations, instances and logs.',
    'dashboard.openFullDashboard': 'Open full dashboard',

    'dashboard.settingsTitle': 'Settings',
    'dashboard.settingsPlaceholder': 'Settings will be implemented here...',

    'dashboard.eventsTitle': 'Events',
    'dashboard.eventsPlaceholder': 'Events monitor will be implemented here...',

    'dashboard.searchPlaceholder': 'Search...',
    'dashboard.refresh': 'Refresh',
    'dashboard.filters': 'Filters',
    'dashboard.moreActions': 'More actions',
    'dashboard.selectedSingular': 'selected',
    'dashboard.selectedPlural': 'selected',
    'dashboard.clear': 'Clear',
    'dashboard.removeFilter': 'Remove filter {label}',
  },
};
