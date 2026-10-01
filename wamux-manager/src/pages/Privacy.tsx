import LegalLayout, { LegalSection } from '@/components/base/LegalLayout';
import { PRODUCT_NAME } from '@/constants/branding';

const UPDATED_AT = '24 de setembro de 2026';

/**
 * Política de Privacidade.
 *
 * Public page linked from the login screen. Explains, in plain terms, which
 * data this self-hosted manager handles, where it is stored and who is
 * responsible for it (the operator, not the fork maintainers).
 */
export default function Privacy() {
  return (
    <LegalLayout
      title="Política de Privacidade"
      subtitle={`Como o ${PRODUCT_NAME} trata dados em uma instalação auto-hospedada.`}
      updatedAt={UPDATED_AT}
    >
      <LegalSection title="1. Resumo">
        <p>
          No {PRODUCT_NAME}, os dados ficam na infraestrutura de quem hospeda o
          sistema — o operador. Este fork não ativa licenças, não envia
          heartbeat e não faz telemetria para servidores da Evolution Foundation
          ou dos mantenedores.
        </p>
      </LegalSection>

      <LegalSection title="2. Quais dados o sistema trata">
        <ul className="list-disc space-y-1 pl-5">
          <li>
            <span className="text-foreground">Autenticação:</span> a{' '}
            <code className="text-foreground">GLOBAL_API_KEY</code> configurada
            no servidor e os tokens por instância. No navegador, a URL da API e
            a chave usada no login podem ser guardadas no armazenamento local
            para manter a sessão.
          </li>
          <li>
            <span className="text-foreground">Instâncias:</span> identificadores,
            nome, estado de conexão, número/JID pareado e metadados de operação.
          </li>
          <li>
            <span className="text-foreground">Mensagens e contatos:</span> quando
            a persistência está habilitada (
            <code className="text-foreground">DATABASE_SAVE_MESSAGES</code>),
            mensagens enviadas e recebidas e metadados de conversa são gravados
            no banco; logs técnicos registram a operação do serviço.
          </li>
        </ul>
      </LegalSection>

      <LegalSection title="3. Onde os dados ficam">
        <p>
          Em um banco PostgreSQL e em volumes locais do servidor do operador.
          Nada disso é enviado aos mantenedores deste fork. Para falar com o
          WhatsApp, o servidor estabelece conexão direta com a infraestrutura do
          WhatsApp/Meta, como parte do protocolo.
        </p>
      </LegalSection>

      <LegalSection title="4. Finalidades e base legal">
        <p>
          Os dados são tratados para operar o serviço: autenticar o acesso,
          conectar instâncias, enviar e receber mensagens e exibir o painel. O
          operador, na condição de controlador, deve definir a base legal
          adequada — por exemplo, execução de contrato, consentimento ou
          legítimo interesse — conforme a LGPD/GDPR e a legislação local.
        </p>
      </LegalSection>

      <LegalSection title="5. Compartilhamento">
        <p>
          Não há venda de dados. O compartilhamento ocorre apenas com o
          WhatsApp/Meta, por ser inerente ao funcionamento do protocolo, e com
          terceiros que o operador decidir integrar.
        </p>
      </LegalSection>

      <LegalSection title="6. Segurança">
        <p>
          Mantenha a <code className="text-foreground">GLOBAL_API_KEY</code> em
          segredo, sirva o painel e a API por HTTPS, restrinja o acesso de rede
          e desative o Swagger público (
          <code className="text-foreground">SWAGGER_ENABLED=false</code>) em
          hosts expostos à internet. Essas medidas ficam a cargo do operador.
        </p>
      </LegalSection>

      <LegalSection title="7. Retenção e exclusão">
        <p>
          As mensagens persistidas são podadas após o período definido em{' '}
          <code className="text-foreground">MESSAGE_RETENTION_DAYS</code>{' '}
          (<code className="text-foreground">0</code> mantém indefinidamente).
          Instâncias e demais dados podem ser removidos pelo painel ou
          diretamente no banco pelo operador.
        </p>
      </LegalSection>

      <LegalSection title="8. Direitos do titular">
        <p>
          Titulares de dados — por exemplo, contatos que trocam mensagens com as
          instâncias — devem procurar o operador da instalação para exercer seus
          direitos. O operador é o controlador e responde pelas solicitações.
        </p>
      </LegalSection>

      <LegalSection title="9. Observação">
        <p>
          Este texto é um modelo de exemplo para instalações auto-hospedadas.
          Revise e adapte com apoio jurídico antes de usá-lo em produção.
        </p>
      </LegalSection>
    </LegalLayout>
  );
}
