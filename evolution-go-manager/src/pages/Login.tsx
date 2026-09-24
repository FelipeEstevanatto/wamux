import React, { useState, useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  Button,
  Input,
  Label,
  Alert,
  AlertTitle,
  AlertDescription,
} from '@/components/ui';
import { toast } from 'sonner';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { AlertCircle, Eye, EyeOff } from 'lucide-react';

import useAuth from '@/hooks/useAuth';
import { COPYRIGHT_LINE, FORK_DISCLAIMER, FORK_OF_NAME, PRODUCT_NAME } from '@/constants/branding';

export const Login: React.FC = () => {
  const { login, isAuthenticated, apiUrl: defaultApiUrl, apiKey: storedApiKey } = useAuth();
  const navigate = useNavigate();

  const [isLoading, setIsLoading] = useState(false);
  const [loginError, setLoginError] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);

  // If already authenticated, go straight to the dashboard.
  useEffect(() => {
    if (isAuthenticated) {
      navigate('/manager', { replace: true });
    }
  }, [isAuthenticated, navigate]);
  
  // Usar a URL atual do navegador como placeholder
  const currentUrl = window.location.origin;

  // Schema de validação para login
const loginSchema = z.object({
  apiUrl: z
    .string()
      .min(1, { message: 'URL da API é obrigatória' })
      .url({ message: `URL inválida. Use o formato: ${currentUrl}` })
    .refine((url) => url.startsWith('http://') || url.startsWith('https://'), {
      message: 'URL deve começar com http:// ou https://',
    }),
  apiKey: z
    .string()
      .min(1, { message: 'API Key é obrigatória' })
      .min(10, { message: 'API Key deve ter pelo menos 10 caracteres' }),
});

type LoginFormData = z.infer<typeof loginSchema>;

  // Formulário de login
  const loginForm = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      apiUrl: defaultApiUrl,
      apiKey: storedApiKey || '',
    },
  });

  const onLoginSubmit = async (data: LoginFormData) => {
    setIsLoading(true);
    setLoginError('');

    try {
      // Validate the API key against the backend (no license check — this fork
      // removed the licensing entirely).
      await login(data.apiUrl, data.apiKey);

      toast.success('Conectado com sucesso!');

      navigate('/manager', { replace: true });
    } catch (error) {
      console.error('Erro ao fazer login:', error);
      const errorMessage =
        error instanceof Error
          ? error.message
          : 'Erro ao conectar. Verifique a URL e API Key.';

      toast.error('Erro', {
        description: errorMessage,
      });

      setLoginError(errorMessage);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4 bg-gradient-to-t from-primary/20 via-background/95 to-background relative">
      <div className="w-full max-w-md space-y-6">
        {/* Logo */}
        <div className="text-center">
          <h1 className="text-3xl font-bold text-primary">
            {PRODUCT_NAME}
          </h1>
        </div>

        {/* Formulário */}
        <div className="bg-background/80 backdrop-blur-sm border rounded-lg p-6 shadow-lg">
          <div className="space-y-2 mb-6">
            <h2 className="text-2xl font-bold">Entrar na sua conta</h2>
            <p className="text-muted-foreground">Digite suas credenciais para acessar o sistema</p>
          </div>

          {/* Mostrar mensagem de erro da aba login */}
          {loginError && (
            <Alert variant="destructive" className="mb-4">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Erro</AlertTitle>
              <AlertDescription>{loginError}</AlertDescription>
            </Alert>
          )}

          <form onSubmit={loginForm.handleSubmit(onLoginSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="login-apiUrl">URL da API</Label>
              <Input
                id="login-apiUrl"
                type="text"
                placeholder={currentUrl}
                disabled={isLoading}
                {...loginForm.register('apiUrl')}
              />
              {loginForm.formState.errors.apiUrl && (
                <p className="text-destructive text-sm">
                  {loginForm.formState.errors.apiUrl.message}
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="login-apiKey">API Key (GLOBAL_API_KEY)</Label>
              <div className="relative">
                <Input
                  id="login-apiKey"
                  type={showApiKey ? 'text' : 'password'}
                  placeholder="Sua chave de API"
                  disabled={isLoading}
                  className="pr-10"
                  {...loginForm.register('apiKey')}
                />
                <button
                  type="button"
                  onClick={() => setShowApiKey((value) => !value)}
                  aria-label={showApiKey ? 'Ocultar API Key' : 'Mostrar API Key'}
                  title={showApiKey ? 'Ocultar API Key' : 'Mostrar API Key'}
                  className="absolute inset-y-0 right-0 flex items-center px-3 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none"
                >
                  {showApiKey ? (
                    <EyeOff className="h-4 w-4" />
                  ) : (
                    <Eye className="h-4 w-4" />
                  )}
                </button>
              </div>
              {loginForm.formState.errors.apiKey && (
                <p className="text-destructive text-sm">
                  {loginForm.formState.errors.apiKey.message}
                </p>
              )}
            </div>

            <div className="text-xs text-muted-foreground">
              <p>
                <strong>Dica:</strong> A API Key é o valor da variável{' '}
                <code className="relative rounded bg-muted px-[0.3rem] py-[0.2rem] font-mono text-xs">
                  GLOBAL_API_KEY
                </code>{' '}
                configurada no arquivo .env do {PRODUCT_NAME}.
              </p>
            </div>

            <Button type="submit" disabled={isLoading} className="w-full">
              {isLoading ? 'Conectando...' : 'Entrar'}
            </Button>
          </form>
        </div>

        {/* Mensagem de termos de serviço */}
        <div className="text-center text-xs text-muted-foreground">
          <p>
            Ao continuar, você concorda com nossos{' '}
            <Link to="/manager/terms" className="underline hover:text-primary">
              Termos de Serviço
            </Link>{' '}
            e{' '}
            <Link to="/manager/privacy" className="underline hover:text-primary">
              Política de Privacidade
            </Link>
            .
          </p>
          <p className="mt-3">{COPYRIGHT_LINE}</p>
          <p className="mt-1">
            {PRODUCT_NAME} é um fork comunitário não oficial do {FORK_OF_NAME}.
          </p>
          <p className="mt-1">{FORK_DISCLAIMER}</p>
        </div>
      </div>
    </div>
  );
};

export default Login;
