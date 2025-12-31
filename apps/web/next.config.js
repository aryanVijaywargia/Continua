/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  transpilePackages: ['@continua/api-client'],
};

module.exports = nextConfig;
