/** Translation namespace for the live events page. Prefixed keys. */
export const eventsNs: {
  pt: Record<string, string>;
  en: Record<string, string>;
} = {
  pt: {
    'events.title': 'Eventos',
    'events.subtitle':
      'Eventos ao vivo. Só mostra o que chega com esta página aberta, e somente os tipos que cada instância assina.',
    'events.allInstances': 'Todas as instâncias',
    'events.instanceFilter': 'Filtrar por instância',
    'events.search': 'Buscar no conteúdo dos eventos',
    'events.typeFilter': 'Filtrar por tipo',
    'events.live': 'Ao vivo',
    'events.connecting': 'Conectando…',
    'events.pause': 'Pausar',
    'events.resume': 'Retomar',
    'events.resumeCount': 'Retomar ({count} novos)',
    'events.clear': 'Limpar',
    'events.showing': '{shown} de {total} eventos',
    'events.waiting': 'Aguardando eventos…',
    'events.noMatch': 'Nenhum evento corresponde aos filtros.',
    'events.copy': 'Copiar JSON',
    'events.copied': 'JSON copiado',
    'events.copyError': 'Não foi possível copiar',
    'events.redacted':
      'Tokens e chaves são ocultados, e conteúdos longos como base64 são cortados.',
    'events.wsDisabledTitle': 'WebSocket desativado',
    'events.wsDisabledBody':
      'Estas instâncias não enviam eventos para esta página. Ative o WebSocket nas configurações de cada uma:',
  },
  en: {
    'events.title': 'Events',
    'events.subtitle':
      'Live events. Shows only what arrives while this page is open, and only the types each instance subscribes to.',
    'events.allInstances': 'All instances',
    'events.instanceFilter': 'Filter by instance',
    'events.search': 'Search event contents',
    'events.typeFilter': 'Filter by type',
    'events.live': 'Live',
    'events.connecting': 'Connecting…',
    'events.pause': 'Pause',
    'events.resume': 'Resume',
    'events.resumeCount': 'Resume ({count} new)',
    'events.clear': 'Clear',
    'events.showing': '{shown} of {total} events',
    'events.waiting': 'Waiting for events…',
    'events.noMatch': 'No events match the filters.',
    'events.copy': 'Copy JSON',
    'events.copied': 'JSON copied',
    'events.copyError': 'Could not copy',
    'events.redacted': 'Tokens and keys are hidden, and long content such as base64 is cut.',
    'events.wsDisabledTitle': 'WebSocket is off',
    'events.wsDisabledBody':
      'These instances send no events to this page. Turn on WebSocket in the settings of each one:',
  },
};
