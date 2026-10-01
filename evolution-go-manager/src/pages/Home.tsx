import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui';
import { ArrowRight, Zap, Shield, Globe, Server } from 'lucide-react';
import apiClient from '@/services/api/client';
import {
  COPYRIGHT_LINE,
  FORK_DISCLAIMER,
  FORK_OF_NAME,
  PRODUCT_NAME,
  PRODUCT_TAGLINE,
} from '@/constants/branding';

interface RootInfo {
  status?: number;
  message?: string;
  version?: string;
  clientName?: string;
  manager?: string;
  documentation?: string;
  whatsappWebVersion?: string;
  error?: string;
}

export const Home: React.FC = () => {
  const navigate = useNavigate();
  const [info, setInfo] = useState<RootInfo | null>(null);

  // The public GET / banner is a good "is the service healthy?" summary, so the
  // landing page doubles as a status view (and surfaces a boot config error).
  useEffect(() => {
    apiClient
      .get<RootInfo>('/')
      .then((res) => setInfo(res.data))
      .catch(() => setInfo(null));
  }, []);

  return (
    <div className="min-h-screen bg-gradient-to-br from-primary/10 via-background to-background">
      {/* Hero Section */}
      <div className="container mx-auto px-4 py-16">
        <div className="max-w-4xl mx-auto text-center space-y-8">
          {/* Logo/Title */}
          <div className="space-y-4">
            <h1 className="text-6xl font-bold text-primary animate-fadeIn">
              {PRODUCT_NAME}
            </h1>
            <p className="text-xl text-muted-foreground">
              {PRODUCT_TAGLINE}
            </p>
            <p className="text-sm text-muted-foreground max-w-2xl mx-auto">
              Fork comunitário não oficial do {FORK_OF_NAME}. Não é afiliado,
              endossado ou uma release oficial da Evolution Foundation.
            </p>
          </div>

          {/* Live service info */}
          {info && (
            <div className="mx-auto max-w-2xl rounded-lg border bg-card p-4 text-left text-sm">
              <div className="mb-2 flex items-center gap-2 font-medium">
                <Server className="h-4 w-4 text-primary" />
                Status do serviço
              </div>
              <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-muted-foreground">
                <dt>Versão</dt>
                <dd className="text-foreground">{info.version || '—'}</dd>
                <dt>Cliente</dt>
                <dd className="text-foreground">{info.clientName || '—'}</dd>
                <dt>WhatsApp Web</dt>
                <dd className="text-foreground">{info.whatsappWebVersion || '—'}</dd>
                <dt>Docs</dt>
                <dd>
                  <a className="text-primary underline" href={info.documentation || '/swagger/index.html'}>
                    {info.documentation || '/swagger/index.html'}
                  </a>
                </dd>
              </dl>
              {info.error && (
                <p className="mt-2 rounded-md bg-destructive/10 p-2 text-destructive">
                  Atenção: {info.error}
                </p>
              )}
            </div>
          )}

          {/* Features */}
          <div className="grid md:grid-cols-3 gap-6 mt-12">
            <div className="bg-card border rounded-lg p-6 space-y-3 hover:border-primary transition-colors">
              <div className="w-12 h-12 bg-primary/10 rounded-lg flex items-center justify-center">
                <Zap className="w-6 h-6 text-primary" />
              </div>
              <h3 className="text-lg font-semibold">Rápido e Eficiente</h3>
              <p className="text-sm text-muted-foreground">
                Gerencie múltiplas instâncias WhatsApp com alta performance
              </p>
            </div>

            <div className="bg-card border rounded-lg p-6 space-y-3 hover:border-primary transition-colors">
              <div className="w-12 h-12 bg-primary/10 rounded-lg flex items-center justify-center">
                <Shield className="w-6 h-6 text-primary" />
              </div>
              <h3 className="text-lg font-semibold">Seguro</h3>
              <p className="text-sm text-muted-foreground">
                Autenticação robusta e controle total sobre suas instâncias
              </p>
            </div>

            <div className="bg-card border rounded-lg p-6 space-y-3 hover:border-primary transition-colors">
              <div className="w-12 h-12 bg-primary/10 rounded-lg flex items-center justify-center">
                <Globe className="w-6 h-6 text-primary" />
              </div>
              <h3 className="text-lg font-semibold">API Completa</h3>
              <p className="text-sm text-muted-foreground">
                Integração via API REST com o {PRODUCT_NAME}
              </p>
            </div>
          </div>

          {/* CTA Button */}
          <div className="mt-12">
            <Button
              size="lg"
              onClick={() => navigate('/manager/login')}
              className="text-lg px-8 py-6 group"
            >
              Acessar Manager
              <ArrowRight className="ml-2 w-5 h-5 group-hover:translate-x-1 transition-transform" />
            </Button>
          </div>

          {/* Footer Info */}
          <div className="mt-16 pt-8 border-t">
            <p className="text-sm text-muted-foreground">
              {PRODUCT_NAME} Manager — gerencie suas instâncias WhatsApp de forma
              simples e eficiente
            </p>
            <p className="text-xs text-muted-foreground mt-2">
              {COPYRIGHT_LINE}
            </p>
            <p className="text-xs text-muted-foreground mt-1 max-w-xl mx-auto">
              {FORK_DISCLAIMER}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};

export default Home;
