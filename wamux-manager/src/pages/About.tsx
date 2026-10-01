import { Info, ShieldCheck, Scale } from 'lucide-react';
import GithubIcon from '@/components/base/GithubIcon';
import {
  COPYRIGHT_LINE,
  PRODUCT_NAME,
  PRODUCT_TAGLINE,
  REPO,
} from '@/constants/branding';

/**
 * About / "Sobre".
 *
 * Also serves as the administrator-visible "this system uses WaMux" notice.
 */
export default function About() {
  return (
    <div className="h-full overflow-y-auto p-4 sm:p-6">
      <div className="mx-auto max-w-3xl space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Sobre</h1>
          <p className="text-sm text-muted-foreground">
            {PRODUCT_NAME} — {PRODUCT_TAGLINE}
          </p>
        </div>

        {/* Administrator-visible notice that this system uses WaMux. */}
        <div className="rounded-xl border border-primary/40 bg-primary/5 p-5">
          <div className="flex items-start gap-3">
            <ShieldCheck className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
            <div className="space-y-1">
              <h2 className="font-semibold text-foreground">
                Este sistema utiliza o {PRODUCT_NAME}
              </h2>
              <p className="text-sm text-muted-foreground">
                O painel e a API que você está usando são o {PRODUCT_NAME}, um
                servidor auto-hospedado que expõe uma API REST sobre o WhatsApp.
              </p>
            </div>
          </div>
        </div>

        {/* License. */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-5">
          <div className="flex items-start gap-3">
            <Scale className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
            <div className="space-y-2">
              <h2 className="font-semibold text-foreground">Licença</h2>
              <p className="text-sm text-muted-foreground">
                Apache License 2.0. Os textos completos estão em{' '}
                <code className="text-foreground">LICENSE</code>,{' '}
                <code className="text-foreground">NOTICE</code> e{' '}
                <code className="text-foreground">TRADEMARKS.md</code> na raiz
                do projeto — e dentro da imagem em{' '}
                <code className="text-foreground">/app</code>.
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
                Sobre o {PRODUCT_NAME}
              </h2>
              <p className="text-sm text-muted-foreground">
                Servidor auto-hospedado com painel versionado, correções de
                segurança, auditoria da API do whatsmeow e recursos extras
                (visão por instância, proxy, timer de mensagens temporárias,
                Typebot, dashboard). O que mudou está em{' '}
                <code className="text-foreground">FORK_NOTES.md</code> e{' '}
                <code className="text-foreground">CHANGELOG.md</code>.
              </p>
              <a
                className="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
                href={REPO}
                target="_blank"
                rel="noreferrer noopener"
              >
                <GithubIcon className="h-4 w-4" />
                Repositório do projeto
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
