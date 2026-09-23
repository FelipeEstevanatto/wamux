import { Info, ShieldCheck, Scale, GitFork } from 'lucide-react';
import GithubIcon from '@/components/base/GithubIcon';
import {
  COPYRIGHT_LINE,
  FORK_DISCLAIMER,
  FORK_DISCLAIMER_EN,
  FORK_OF_NAME,
  FORK_REPO,
  PRODUCT_NAME,
  PRODUCT_TAGLINE,
  UPSTREAM_REPO,
} from '@/constants/branding';

/**
 * About / "Sobre".
 *
 * The Evolution Go license (LICENSE §1.b) requires that any system using it
 * shows a clear, administrator-visible notice that Evolution Go is in use, and
 * that it is reachable from the documentation or a settings page. This page is
 * that notice — stated first, before anything about this fork.
 */
export default function About() {
  return (
    <div className="h-full overflow-y-auto p-6">
      <div className="mx-auto max-w-3xl space-y-6">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Sobre</h1>
          <p className="text-sm text-muted-foreground">
            {PRODUCT_NAME} — {PRODUCT_TAGLINE}
          </p>
        </div>

        {/* The required usage notification, stated plainly and first. */}
        <div className="rounded-xl border border-primary/40 bg-primary/5 p-5">
          <div className="flex items-start gap-3">
            <ShieldCheck className="mt-0.5 h-5 w-5 flex-shrink-0 text-primary" />
            <div className="space-y-1">
              <h2 className="font-semibold text-foreground">
                Este sistema utiliza o {FORK_OF_NAME}
              </h2>
              <p className="text-sm text-muted-foreground">
                O painel e a API que você está usando são construídos sobre o{' '}
                {FORK_OF_NAME}, um projeto de código aberto da Evolution
                Foundation. O {PRODUCT_NAME} é um fork comunitário dele.
              </p>
            </div>
          </div>
        </div>

        {/* Fork disclaimer — the point of this page. */}
        <div className="rounded-xl border border-amber-500/40 bg-amber-500/5 p-5">
          <div className="flex items-start gap-3">
            <GitFork className="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-500" />
            <div className="space-y-2">
              <h2 className="font-semibold text-foreground">
                Não é oficial
              </h2>
              <p className="text-sm text-muted-foreground">
                {FORK_DISCLAIMER}
              </p>
              <p className="text-xs text-muted-foreground">
                {FORK_DISCLAIMER_EN}
              </p>
              <div className="flex flex-wrap gap-4 pt-1 text-sm">
                <a
                  className="inline-flex items-center gap-1.5 text-primary hover:underline"
                  href={FORK_REPO}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  <GithubIcon className="h-4 w-4" />
                  Repositório deste fork
                </a>
                <a
                  className="inline-flex items-center gap-1.5 text-muted-foreground hover:text-foreground"
                  href={UPSTREAM_REPO}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  <GithubIcon className="h-4 w-4" />
                  Projeto upstream ({FORK_OF_NAME})
                </a>
              </div>
            </div>
          </div>
        </div>

        {/* License. */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-5">
          <div className="flex items-start gap-3">
            <Scale className="mt-0.5 h-5 w-5 flex-shrink-0 text-muted-foreground" />
            <div className="space-y-2">
              <h2 className="font-semibold text-foreground">Licença</h2>
              <p className="text-sm text-muted-foreground">
                Apache License 2.0, com condições adicionais do {FORK_OF_NAME}{' '}
                (notificação de uso e uso de marca). Os textos completos estão em{' '}
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

        {/* About this fork. */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-5">
          <div className="flex items-start gap-3">
            <Info className="mt-0.5 h-5 w-5 flex-shrink-0 text-muted-foreground" />
            <div className="space-y-2">
              <h2 className="font-semibold text-foreground">
                Sobre o {PRODUCT_NAME}
              </h2>
              <p className="text-sm text-muted-foreground">
                Fork comunitário do {FORK_OF_NAME} com o painel (fonte
                versionada), correções de segurança, auditoria da API do
                whatsmeow e recursos extras (visão por instância, proxy, timer de
                mensagens temporárias, Typebot, dashboard). O que mudou está em{' '}
                <code className="text-foreground">FORK_NOTES.md</code> e{' '}
                <code className="text-foreground">CHANGELOG.md</code>.
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
