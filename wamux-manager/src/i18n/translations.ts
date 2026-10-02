/**
 * UI strings.
 *
 * Flat, dotted keys so lookups are a single map access and new languages are a
 * copy of the object. Portuguese (pt) is the source of truth and the fallback;
 * anything missing in another language degrades to pt rather than showing a key.
 *
 * Add a language by adding a key here and an option in the language switcher.
 */
export type Lang = 'pt' | 'en';

export const LANGUAGES: { id: Lang; label: string }[] = [
  { id: 'pt', label: 'Português' },
  { id: 'en', label: 'English' },
];

import { dashboardNs } from './namespaces/dashboard';
import { instancesNs } from './namespaces/instances';
import { messagesNs } from './namespaces/messages';
import { instanceSettingsNs } from './namespaces/instanceSettings';
import { apiTesterNs } from './namespaces/apiTester';
import { modalsNs } from './namespaces/modals';
import { legalNs } from './namespaces/legal';

const basePt: Record<string, string> = {
  'common.cancel': 'Cancelar',
  'common.close': 'Fechar',
  'common.confirm': 'Confirmar',
  'common.loading': 'Carregando…',
  'common.retry': 'Tentar novamente',
  'common.reload': 'Recarregar página',
  'common.home': 'Voltar ao início',
  'common.copied': 'Copiado!',
  'common.copy': 'Copiar',
  'common.error': 'Erro',

  'sidebar.dashboard': 'Dashboard',
  'sidebar.instances': 'Instâncias',
  'sidebar.messages': 'Mensagens',
  'sidebar.apiTester': 'API Tester',
  'sidebar.about': 'Sobre',
  'sidebar.swagger': 'Swagger',
  'sidebar.swaggerTitle': 'Abrir o Swagger da API em nova aba',
  'sidebar.version': 'versão {version}',
  'sidebar.versionEmpty': 'versão —',
  'sidebar.github': 'GitHub',
  'sidebar.repoTitle': 'Repositório do projeto',
  'sidebar.homeTitle': 'Ir para o Dashboard',

  'header.toggleTheme': 'Alternar tema',
  'header.lightMode': 'Ativar modo claro',
  'header.darkMode': 'Ativar modo escuro',
  'header.logout': 'Sair',
  'header.openMenu': 'Abrir menu',
  'header.language': 'Idioma',

  'banner.offline': 'Sem conexão com a internet.',
  'banner.reconnecting': 'Reconectando…',
  'banner.backOnline': 'Conexão restabelecida.',

  'error.title': 'Algo deu errado',
  'error.subtitle': 'Ocorreu um erro inesperado nesta tela.',

  'palette.placeholder': 'Buscar páginas, instâncias e ações…',
  'palette.noResults': 'Nenhum resultado.',
  'palette.pages': 'Páginas',
  'palette.instances': 'Instâncias',
  'palette.actions': 'Ações',
  'palette.toggleTheme': 'Alternar tema',
  'palette.logout': 'Sair',
  'palette.openSwagger': 'Abrir Swagger',
  'palette.openDashboard': 'Ir para o Dashboard',
  'palette.hint': 'Atalho: Ctrl/⌘ + K',

  'home.tagline': 'API WhatsApp self-hosted para gerenciar instâncias',
  'home.cta': 'Acessar Manager',
  'home.statusTitle': 'Status do serviço',
  'home.version': 'Versão',
  'home.client': 'Cliente',
  'home.whatsappWeb': 'WhatsApp Web',
  'home.docs': 'Docs',
  'home.attention': 'Atenção: {error}',
  'home.f1.title': 'Rápido e eficiente',
  'home.f1.desc': 'Gerencie múltiplas instâncias WhatsApp com alta performance',
  'home.f2.title': 'Seguro',
  'home.f2.desc': 'Autenticação robusta e controle total sobre suas instâncias',
  'home.f3.title': 'API completa',
  'home.f3.desc': 'Integração via API REST com o {product}',
  'home.footer': '{product} Manager — gerencie suas instâncias WhatsApp de forma simples e eficiente',

  'login.title': 'Entrar no Manager',
  'login.apiUrl': 'URL da API',
  'login.apiKey': 'API Key',
  'login.submit': 'Entrar',
  'login.submitting': 'Conectando…',
  'login.tip': 'A API Key é o valor de GLOBAL_API_KEY no arquivo .env do {product}.',
  'login.termsPrefix': 'Ao continuar, você concorda com nossos',
  'login.terms': 'Termos de Serviço',
  'login.and': 'e',
  'login.privacy': 'Política de Privacidade',
  'login.heading': 'Entrar na sua conta',
  'login.subheading': 'Digite suas credenciais para acessar o sistema',
  'login.showKey': 'Mostrar API Key',
  'login.hideKey': 'Ocultar API Key',
  'login.apiKeyPlaceholder': 'Sua chave de API',
  'login.success': 'Conectado com sucesso!',
  'login.errorFallback': 'Erro ao conectar. Verifique a URL e API Key.',

  'about.title': 'Sobre',
  'about.noticeTitle': 'Este sistema utiliza o {product}',
  'about.noticeBody':
    'O painel e a API que você está usando são o {product}, um servidor auto-hospedado que expõe uma API REST sobre o WhatsApp.',
  'about.licenseTitle': 'Licença',
  'about.licenseBody':
    'Apache License 2.0. Os textos completos estão em LICENSE, NOTICE e TRADEMARKS.md na raiz do projeto — e dentro da imagem em /app.',
  'about.projectTitle': 'Sobre o {product}',
  'about.projectBody':
    'Servidor auto-hospedado com painel versionado, correções de segurança, auditoria da API do whatsmeow e recursos extras (visão por instância, proxy, timer de mensagens temporárias, Typebot, dashboard). O que mudou está em FORK_NOTES.md e CHANGELOG.md.',
  'about.repo': 'Repositório do projeto',

  'common.navigationMenu': 'Menu de navegação',
  'legal.backToLogin': 'Voltar para o login',
  'legal.updatedAt': 'Última atualização: {date}',
  'sendMessage.title': 'Enviar mensagem',
  'sendMessage.number': 'Número (com DDI)',
  'sendMessage.numberPlaceholder': '5511999999999',
  'sendMessage.message': 'Mensagem',
  'sendMessage.messagePlaceholder': 'Digite sua mensagem...',
  'sendMessage.submit': 'Enviar',
  'sendMessage.submitting': 'Enviando…',
  'sendMessage.tokenMissing': 'Token da instância não encontrado',
  'sendMessage.success': 'Mensagem enviada com sucesso!',
  'sendMessage.error': 'Erro ao enviar mensagem',
};

const baseEn: Record<string, string> = {
  'common.cancel': 'Cancel',
  'common.close': 'Close',
  'common.confirm': 'Confirm',
  'common.loading': 'Loading…',
  'common.retry': 'Try again',
  'common.reload': 'Reload page',
  'common.home': 'Back to home',
  'common.copied': 'Copied!',
  'common.copy': 'Copy',
  'common.error': 'Error',

  'sidebar.dashboard': 'Dashboard',
  'sidebar.instances': 'Instances',
  'sidebar.messages': 'Messages',
  'sidebar.apiTester': 'API Tester',
  'sidebar.about': 'About',
  'sidebar.swagger': 'Swagger',
  'sidebar.swaggerTitle': 'Open the API Swagger in a new tab',
  'sidebar.version': 'version {version}',
  'sidebar.versionEmpty': 'version —',
  'sidebar.github': 'GitHub',
  'sidebar.repoTitle': 'Project repository',
  'sidebar.homeTitle': 'Go to the Dashboard',

  'header.toggleTheme': 'Toggle theme',
  'header.lightMode': 'Switch to light mode',
  'header.darkMode': 'Switch to dark mode',
  'header.logout': 'Sign out',
  'header.openMenu': 'Open menu',
  'header.language': 'Language',

  'banner.offline': 'No internet connection.',
  'banner.reconnecting': 'Reconnecting…',
  'banner.backOnline': 'Connection restored.',

  'error.title': 'Something went wrong',
  'error.subtitle': 'An unexpected error occurred on this screen.',

  'palette.placeholder': 'Search pages, instances and actions…',
  'palette.noResults': 'No results.',
  'palette.pages': 'Pages',
  'palette.instances': 'Instances',
  'palette.actions': 'Actions',
  'palette.toggleTheme': 'Toggle theme',
  'palette.logout': 'Sign out',
  'palette.openSwagger': 'Open Swagger',
  'palette.openDashboard': 'Go to the Dashboard',
  'palette.hint': 'Shortcut: Ctrl/⌘ + K',

  'home.tagline': 'Self-hosted WhatsApp API to manage instances',
  'home.cta': 'Open Manager',
  'home.statusTitle': 'Service status',
  'home.version': 'Version',
  'home.client': 'Client',
  'home.whatsappWeb': 'WhatsApp Web',
  'home.docs': 'Docs',
  'home.attention': 'Warning: {error}',
  'home.f1.title': 'Fast and efficient',
  'home.f1.desc': 'Manage multiple WhatsApp instances with high performance',
  'home.f2.title': 'Secure',
  'home.f2.desc': 'Robust authentication and full control over your instances',
  'home.f3.title': 'Complete API',
  'home.f3.desc': 'REST API integration with {product}',
  'home.footer': '{product} Manager — manage your WhatsApp instances simply and efficiently',

  'login.title': 'Sign in to the Manager',
  'login.apiUrl': 'API URL',
  'login.apiKey': 'API Key',
  'login.submit': 'Sign in',
  'login.submitting': 'Connecting…',
  'login.tip': 'The API Key is the GLOBAL_API_KEY value in the {product} .env file.',
  'login.termsPrefix': 'By continuing you agree to our',
  'login.terms': 'Terms of Service',
  'login.and': 'and',
  'login.privacy': 'Privacy Policy',
  'login.heading': 'Sign in to your account',
  'login.subheading': 'Enter your credentials to access the system',
  'login.showKey': 'Show API Key',
  'login.hideKey': 'Hide API Key',
  'login.apiKeyPlaceholder': 'Your API key',
  'login.success': 'Connected successfully!',
  'login.errorFallback': 'Connection failed. Check the URL and API Key.',

  'about.title': 'About',
  'about.noticeTitle': 'This system uses {product}',
  'about.noticeBody':
    'The panel and API you are using are {product}, a self-hosted server that exposes a REST API over WhatsApp.',
  'about.licenseTitle': 'License',
  'about.licenseBody':
    'Apache License 2.0. The full texts live in LICENSE, NOTICE and TRADEMARKS.md at the repository root — and inside the image under /app.',
  'about.projectTitle': 'About {product}',
  'about.projectBody':
    'Self-hosted server with a versioned panel, security fixes, a whatsmeow API audit and extra features (per-instance overview, proxy, ephemeral-message timer, Typebot, dashboard). What changed is in FORK_NOTES.md and CHANGELOG.md.',
  'about.repo': 'Project repository',

  'common.navigationMenu': 'Navigation menu',
  'legal.backToLogin': 'Back to login',
  'legal.updatedAt': 'Last updated: {date}',
  'sendMessage.title': 'Send message',
  'sendMessage.number': 'Number (with country code)',
  'sendMessage.numberPlaceholder': '5511999999999',
  'sendMessage.message': 'Message',
  'sendMessage.messagePlaceholder': 'Type your message...',
  'sendMessage.submit': 'Send',
  'sendMessage.submitting': 'Sending…',
  'sendMessage.tokenMissing': 'Instance token not found',
  'sendMessage.success': 'Message sent successfully!',
  'sendMessage.error': 'Error sending message',
};

export const translations: Record<Lang, Record<string, string>> = {
  pt: {
    ...basePt,
    ...dashboardNs.pt,
    ...instancesNs.pt,
    ...messagesNs.pt,
    ...instanceSettingsNs.pt,
    ...apiTesterNs.pt,
    ...modalsNs.pt,
    ...legalNs.pt,
  },
  en: {
    ...baseEn,
    ...dashboardNs.en,
    ...instancesNs.en,
    ...messagesNs.en,
    ...instanceSettingsNs.en,
    ...apiTesterNs.en,
    ...modalsNs.en,
    ...legalNs.en,
  },
};
