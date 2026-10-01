import LegalLayout, { LegalSection } from '@/components/base/LegalLayout';
import {
  FORK_OF_NAME,
  FORK_REPO,
  PRODUCT_NAME,
  UPSTREAM_REPO,
} from '@/constants/branding';

const UPDATED_AT = '24 de setembro de 2026';

/**
 * Termos de Serviço.
 *
 * Public page linked from the login screen. Describes what this self-hosted
 * WhatsApp instance manager actually does and the rules for using it.
 */
export default function Terms() {
  return (
    <LegalLayout
      title="Termos de Serviço"
      subtitle={`Condições de uso do ${PRODUCT_NAME} Manager.`}
      updatedAt={UPDATED_AT}
    >
      <LegalSection title="1. Sobre este software">
        <p>
          O {PRODUCT_NAME} é um fork comunitário não oficial do {FORK_OF_NAME}{' '}
          distribuído sob a Apache License 2.0. Trata-se de um servidor
          auto-hospedado que expõe uma API REST e este painel web para criar,
          conectar e gerenciar instâncias do WhatsApp, enviar e receber
          mensagens e acompanhar eventos.
        </p>
        <p>
          O software não é afiliado, endossado ou uma release oficial do
          WhatsApp/Meta, nem da Evolution Foundation.
        </p>
      </LegalSection>

      <LegalSection title="2. Aceitação destes termos">
        <p>
          Ao acessar este painel, usar a API ou conectar uma instância, você
          concorda com estes termos. Se não concordar, não utilize o sistema.
        </p>
        <p>
          Eles se aplicam ao operador da instalação — quem hospeda o servidor —
          e a todos os usuários que ele autorizar.
        </p>
      </LegalSection>

      <LegalSection title="3. Uso aceitável">
        <ul className="list-disc space-y-1 pl-5">
          <li>
            Cumprir os Termos de Serviço do WhatsApp/Meta e a legislação
            aplicável, inclusive as regras de proteção de dados.
          </li>
          <li>
            Não enviar spam, mensagens não solicitadas em massa ou conteúdo
            ilícito, abusivo ou enganoso.
          </li>
          <li>Não usar o sistema para violar direitos de terceiros.</li>
        </ul>
        <p>
          O operador é o único responsável pelo conteúdo enviado e pelos
          contatos utilizados.
        </p>
      </LegalSection>

      <LegalSection title="4. Credenciais e acesso">
        <p>
          A <code className="text-foreground">GLOBAL_API_KEY</code> e os tokens
          de instância são credenciais administrativas. Mantenha-as em segredo,
          use HTTPS e restrinja o acesso de rede ao servidor. Quem possui essas
          chaves tem controle total sobre as instâncias.
        </p>
      </LegalSection>

      <LegalSection title="5. Natureza não oficial e riscos">
        <p>
          Este é um cliente não oficial do WhatsApp baseado no protocolo
          multi-dispositivo. O WhatsApp pode alterar ou bloquear clientes não
          oficiais a qualquer momento. O uso é por sua conta e risco, e podem
          ocorrer desconexões, bloqueio de número ou perda de mensagens.
        </p>
      </LegalSection>

      <LegalSection title="6. Ausência de garantias">
        <p>
          O software é fornecido “como está”, sem garantias de qualquer tipo,
          expressas ou implícitas, incluindo adequação a um propósito específico
          ou não violação, conforme os termos da Apache License 2.0.
        </p>
      </LegalSection>

      <LegalSection title="7. Limitação de responsabilidade">
        <p>
          Na extensão máxima permitida pela lei, os mantenedores deste fork e os
          autores do projeto upstream não respondem por danos indiretos,
          incidentais ou consequentes, perda de dados, lucros cessantes ou
          bloqueio de contas decorrentes do uso do software.
        </p>
      </LegalSection>

      <LegalSection title="8. Licença e marca">
        <p>
          O código é licenciado sob a Apache License 2.0, com condições
          adicionais do {FORK_OF_NAME} (notificação de uso e uso de marca). Os
          textos completos estão em{' '}
          <code className="text-foreground">LICENSE</code>,{' '}
          <code className="text-foreground">NOTICE</code> e{' '}
          <code className="text-foreground">TRADEMARKS.md</code> na raiz do
          projeto.
        </p>
        <div className="flex flex-wrap gap-4 pt-1">
          <a
            className="text-primary hover:underline"
            href={FORK_REPO}
            target="_blank"
            rel="noreferrer noopener"
          >
            Repositório deste fork
          </a>
          <a
            className="text-muted-foreground hover:text-foreground"
            href={UPSTREAM_REPO}
            target="_blank"
            rel="noreferrer noopener"
          >
            Projeto upstream ({FORK_OF_NAME})
          </a>
        </div>
      </LegalSection>

      <LegalSection title="9. Alterações">
        <p>
          Estes termos podem ser atualizados a qualquer momento. O uso
          continuado após uma alteração significa que você concorda com a versão
          revisada.
        </p>
      </LegalSection>

      <LegalSection title="10. Observação">
        <p>
          Este texto é um modelo de exemplo para instalações auto-hospedadas.
          Revise e adapte com apoio jurídico antes de usá-lo em produção.
        </p>
      </LegalSection>
    </LegalLayout>
  );
}
