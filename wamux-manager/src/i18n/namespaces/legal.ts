/** Translation namespace for the legal pages (Terms / Privacy). Prefixed keys. */
export const legalNs: {
  pt: Record<string, string>;
  en: Record<string, string>;
} = {
  pt: {
    'legal.updatedDate': '24 de setembro de 2026',

    // Termos de Serviço
    'legal.terms.title': 'Termos de Serviço',
    'legal.terms.subtitle': 'Condições de uso do {product} Manager.',

    'legal.terms.s1.title': '1. Sobre este software',
    'legal.terms.s1.p1':
      'O {product} é um servidor auto-hospedado, distribuído sob a Apache License 2.0. Ele expõe uma API REST e este painel web para criar, conectar e gerenciar instâncias do WhatsApp, enviar e receber mensagens e acompanhar eventos.',
    'legal.terms.s1.p2':
      'O software não é afiliado, endossado ou uma release oficial do WhatsApp/Meta.',

    'legal.terms.s2.title': '2. Aceitação destes termos',
    'legal.terms.s2.p1':
      'Ao acessar este painel, usar a API ou conectar uma instância, você concorda com estes termos. Se não concordar, não utilize o sistema.',
    'legal.terms.s2.p2':
      'Eles se aplicam ao operador da instalação — quem hospeda o servidor — e a todos os usuários que ele autorizar.',

    'legal.terms.s3.title': '3. Uso aceitável',
    'legal.terms.s3.li1':
      'Cumprir os Termos de Serviço do WhatsApp/Meta e a legislação aplicável, inclusive as regras de proteção de dados.',
    'legal.terms.s3.li2':
      'Não enviar spam, mensagens não solicitadas em massa ou conteúdo ilícito, abusivo ou enganoso.',
    'legal.terms.s3.li3':
      'Não usar o sistema para violar direitos de terceiros.',
    'legal.terms.s3.p1':
      'O operador é o único responsável pelo conteúdo enviado e pelos contatos utilizados.',

    'legal.terms.s4.title': '4. Credenciais e acesso',
    'legal.terms.s4.p1Pre': 'A',
    'legal.terms.s4.p1Post':
      'e os tokens de instância são credenciais administrativas. Mantenha-as em segredo, use HTTPS e restrinja o acesso de rede ao servidor. Quem possui essas chaves tem controle total sobre as instâncias.',

    'legal.terms.s5.title': '5. Natureza não oficial e riscos',
    'legal.terms.s5.p1':
      'Este é um cliente não oficial do WhatsApp baseado no protocolo multi-dispositivo. O WhatsApp pode alterar ou bloquear clientes não oficiais a qualquer momento. O uso é por sua conta e risco, e podem ocorrer desconexões, bloqueio de número ou perda de mensagens.',

    'legal.terms.s6.title': '6. Ausência de garantias',
    'legal.terms.s6.p1':
      'O software é fornecido “como está”, sem garantias de qualquer tipo, expressas ou implícitas, incluindo adequação a um propósito específico ou não violação, conforme os termos da Apache License 2.0.',

    'legal.terms.s7.title': '7. Limitação de responsabilidade',
    'legal.terms.s7.p1':
      'Na extensão máxima permitida pela lei, os mantenedores do projeto não respondem por danos indiretos, incidentais ou consequentes, perda de dados, lucros cessantes ou bloqueio de contas decorrentes do uso do software.',

    'legal.terms.s8.title': '8. Licença e marca',
    'legal.terms.s8.p1Pre':
      'O código é licenciado sob a Apache License 2.0. Os textos completos estão em',
    'legal.terms.s8.p1Between1': ',',
    'legal.terms.s8.p1Between2': 'e',
    'legal.terms.s8.p1Post': 'na raiz do projeto.',
    'legal.terms.s8.repo': 'Repositório do projeto',

    'legal.terms.s9.title': '9. Alterações',
    'legal.terms.s9.p1':
      'Estes termos podem ser atualizados a qualquer momento. O uso continuado após uma alteração significa que você concorda com a versão revisada.',

    'legal.terms.s10.title': '10. Observação',
    'legal.terms.s10.p1':
      'Este texto é um modelo de exemplo para instalações auto-hospedadas. Revise e adapte com apoio jurídico antes de usá-lo em produção.',

    // Política de Privacidade
    'legal.privacy.title': 'Política de Privacidade',
    'legal.privacy.subtitle':
      'Como o {product} trata dados em uma instalação auto-hospedada.',

    'legal.privacy.s1.title': '1. Resumo',
    'legal.privacy.s1.p1':
      'No {product}, os dados ficam na infraestrutura de quem hospeda o sistema — o operador. O software não ativa licenças, não envia heartbeat e não faz telemetria para servidores de terceiros.',

    'legal.privacy.s2.title': '2. Quais dados o sistema trata',
    'legal.privacy.s2.li1Label': 'Autenticação:',
    'legal.privacy.s2.li1Pre': 'a',
    'legal.privacy.s2.li1Post':
      'configurada no servidor e os tokens por instância. No navegador, a URL da API e a chave usada no login podem ser guardadas no armazenamento local para manter a sessão.',
    'legal.privacy.s2.li2Label': 'Instâncias:',
    'legal.privacy.s2.li2Text':
      'identificadores, nome, estado de conexão, número/JID pareado e metadados de operação.',
    'legal.privacy.s2.li3Label': 'Mensagens e contatos:',
    'legal.privacy.s2.li3Pre': 'quando a persistência está habilitada (',
    'legal.privacy.s2.li3Post':
      '), mensagens enviadas e recebidas e metadados de conversa são gravados no banco; logs técnicos registram a operação do serviço.',

    'legal.privacy.s3.title': '3. Onde os dados ficam',
    'legal.privacy.s3.p1':
      'Em um banco PostgreSQL e em volumes locais do servidor do operador. Nada disso é enviado aos mantenedores deste projeto. Para falar com o WhatsApp, o servidor estabelece conexão direta com a infraestrutura do WhatsApp/Meta, como parte do protocolo.',

    'legal.privacy.s4.title': '4. Finalidades e base legal',
    'legal.privacy.s4.p1':
      'Os dados são tratados para operar o serviço: autenticar o acesso, conectar instâncias, enviar e receber mensagens e exibir o painel. O operador, na condição de controlador, deve definir a base legal adequada — por exemplo, execução de contrato, consentimento ou legítimo interesse — conforme a LGPD/GDPR e a legislação local.',

    'legal.privacy.s5.title': '5. Compartilhamento',
    'legal.privacy.s5.p1':
      'Não há venda de dados. O compartilhamento ocorre apenas com o WhatsApp/Meta, por ser inerente ao funcionamento do protocolo, e com terceiros que o operador decidir integrar.',

    'legal.privacy.s6.title': '6. Segurança',
    'legal.privacy.s6.p1Pre': 'Mantenha a',
    'legal.privacy.s6.p1Mid':
      'em segredo, sirva o painel e a API por HTTPS, restrinja o acesso de rede e desative o Swagger público (',
    'legal.privacy.s6.p1Post':
      ') em hosts expostos à internet. Essas medidas ficam a cargo do operador.',

    'legal.privacy.s7.title': '7. Retenção e exclusão',
    'legal.privacy.s7.p1Pre':
      'As mensagens persistidas são podadas após o período definido em',
    'legal.privacy.s7.p1Mid': '(',
    'legal.privacy.s7.p1Post':
      'mantém indefinidamente). Instâncias e demais dados podem ser removidos pelo painel ou diretamente no banco pelo operador.',

    'legal.privacy.s8.title': '8. Direitos do titular',
    'legal.privacy.s8.p1':
      'Titulares de dados — por exemplo, contatos que trocam mensagens com as instâncias — devem procurar o operador da instalação para exercer seus direitos. O operador é o controlador e responde pelas solicitações.',

    'legal.privacy.s9.title': '9. Observação',
    'legal.privacy.s9.p1':
      'Este texto é um modelo de exemplo para instalações auto-hospedadas. Revise e adapte com apoio jurídico antes de usá-lo em produção.',
  },
  en: {
    'legal.updatedDate': 'September 24, 2026',

    // Terms of Service
    'legal.terms.title': 'Terms of Service',
    'legal.terms.subtitle': 'Conditions of use of {product} Manager.',

    'legal.terms.s1.title': '1. About this software',
    'legal.terms.s1.p1':
      '{product} is a self-hosted server, distributed under the Apache License 2.0. It exposes a REST API and this web panel to create, connect and manage WhatsApp instances, send and receive messages and track events.',
    'legal.terms.s1.p2':
      'The software is not affiliated with, endorsed by or an official release of WhatsApp/Meta.',

    'legal.terms.s2.title': '2. Acceptance of these terms',
    'legal.terms.s2.p1':
      'By accessing this panel, using the API or connecting an instance, you agree to these terms. If you do not agree, do not use the system.',
    'legal.terms.s2.p2':
      'They apply to the operator of the installation — whoever hosts the server — and to all users they authorize.',

    'legal.terms.s3.title': '3. Acceptable use',
    'legal.terms.s3.li1':
      'Comply with the WhatsApp/Meta Terms of Service and applicable legislation, including data protection rules.',
    'legal.terms.s3.li2':
      'Do not send spam, unsolicited bulk messages or unlawful, abusive or misleading content.',
    'legal.terms.s3.li3':
      'Do not use the system to violate the rights of third parties.',
    'legal.terms.s3.p1':
      'The operator is solely responsible for the content sent and the contacts used.',

    'legal.terms.s4.title': '4. Credentials and access',
    'legal.terms.s4.p1Pre': 'The',
    'legal.terms.s4.p1Post':
      'and the instance tokens are administrative credentials. Keep them secret, use HTTPS and restrict network access to the server. Whoever holds these keys has full control over the instances.',

    'legal.terms.s5.title': '5. Unofficial nature and risks',
    'legal.terms.s5.p1':
      'This is an unofficial WhatsApp client based on the multi-device protocol. WhatsApp may change or block unofficial clients at any time. Use is at your own risk, and disconnections, number bans or message loss may occur.',

    'legal.terms.s6.title': '6. No warranties',
    'legal.terms.s6.p1':
      'The software is provided “as is”, without warranties of any kind, express or implied, including fitness for a particular purpose or non-infringement, under the terms of the Apache License 2.0.',

    'legal.terms.s7.title': '7. Limitation of liability',
    'legal.terms.s7.p1':
      'To the maximum extent permitted by law, the project maintainers are not liable for indirect, incidental or consequential damages, data loss, lost profits or account bans arising from the use of the software.',

    'legal.terms.s8.title': '8. License and trademark',
    'legal.terms.s8.p1Pre':
      'The code is licensed under the Apache License 2.0. The full texts are in',
    'legal.terms.s8.p1Between1': ',',
    'legal.terms.s8.p1Between2': 'and',
    'legal.terms.s8.p1Post': 'at the project root.',
    'legal.terms.s8.repo': 'Project repository',

    'legal.terms.s9.title': '9. Changes',
    'legal.terms.s9.p1':
      'These terms may be updated at any time. Continued use after a change means you agree to the revised version.',

    'legal.terms.s10.title': '10. Note',
    'legal.terms.s10.p1':
      'This text is a sample template for self-hosted installations. Review and adapt it with legal support before using it in production.',

    // Privacy Policy
    'legal.privacy.title': 'Privacy Policy',
    'legal.privacy.subtitle':
      'How {product} handles data in a self-hosted installation.',

    'legal.privacy.s1.title': '1. Summary',
    'legal.privacy.s1.p1':
      'In {product}, data stays on the infrastructure of whoever hosts the system — the operator. The software does not activate licenses, does not send heartbeats and does not perform telemetry to third-party servers.',

    'legal.privacy.s2.title': '2. What data the system handles',
    'legal.privacy.s2.li1Label': 'Authentication:',
    'legal.privacy.s2.li1Pre': 'the',
    'legal.privacy.s2.li1Post':
      'configured on the server and the per-instance tokens. In the browser, the API URL and the key used at login may be stored in local storage to keep the session.',
    'legal.privacy.s2.li2Label': 'Instances:',
    'legal.privacy.s2.li2Text':
      'identifiers, name, connection state, paired number/JID and operation metadata.',
    'legal.privacy.s2.li3Label': 'Messages and contacts:',
    'legal.privacy.s2.li3Pre': 'when persistence is enabled (',
    'legal.privacy.s2.li3Post':
      '), sent and received messages and conversation metadata are written to the database; technical logs record the operation of the service.',

    'legal.privacy.s3.title': '3. Where the data is stored',
    'legal.privacy.s3.p1':
      "In a PostgreSQL database and in local volumes on the operator's server. None of this is sent to the maintainers of this project. To talk to WhatsApp, the server establishes a direct connection to the WhatsApp/Meta infrastructure, as part of the protocol.",

    'legal.privacy.s4.title': '4. Purposes and legal basis',
    'legal.privacy.s4.p1':
      'The data is processed to operate the service: authenticate access, connect instances, send and receive messages and display the panel. The operator, as controller, must define the appropriate legal basis — for example, performance of a contract, consent or legitimate interest — in accordance with the LGPD/GDPR and local legislation.',

    'legal.privacy.s5.title': '5. Sharing',
    'legal.privacy.s5.p1':
      'There is no sale of data. Sharing occurs only with WhatsApp/Meta, as it is inherent to the operation of the protocol, and with third parties that the operator decides to integrate.',

    'legal.privacy.s6.title': '6. Security',
    'legal.privacy.s6.p1Pre': 'Keep the',
    'legal.privacy.s6.p1Mid':
      'secret, serve the panel and the API over HTTPS, restrict network access and disable the public Swagger (',
    'legal.privacy.s6.p1Post':
      ") on hosts exposed to the internet. These measures are the operator's responsibility.",

    'legal.privacy.s7.title': '7. Retention and deletion',
    'legal.privacy.s7.p1Pre':
      'Persisted messages are pruned after the period defined in',
    'legal.privacy.s7.p1Mid': '(',
    'legal.privacy.s7.p1Post':
      'keeps them indefinitely). Instances and other data can be removed through the panel or directly in the database by the operator.',

    'legal.privacy.s8.title': '8. Data subject rights',
    'legal.privacy.s8.p1':
      'Data subjects — for example, contacts who exchange messages with the instances — must contact the operator of the installation to exercise their rights. The operator is the controller and is responsible for the requests.',

    'legal.privacy.s9.title': '9. Note',
    'legal.privacy.s9.p1':
      'This text is a sample template for self-hosted installations. Review and adapt it with legal support before using it in production.',
  },
};
