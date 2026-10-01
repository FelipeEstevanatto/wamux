-- Script de inicialização dos bancos de dados WaMux

-- Criar database para autenticação
CREATE DATABASE wamux_auth;

-- Criar database para dados de usuários
CREATE DATABASE wamux_users;

-- Mensagem de confirmação
SELECT 'Databases wamux_auth e wamux_users criados com sucesso!' as message;
