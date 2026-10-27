export const dnsProviders = {
  cloudflare: {
    name: 'Cloudflare',
    tokenKey: 'dns_api_token', secretKey: '', sessionKey: '', zoneKey: 'dns_zone',
    tokenLabel: 'Cloudflare API Token', zoneLabel: 'Cloudflare Zone ID',
    zonePlaceholder: 'e.g. 023e105f4ecef8ad9ca31a8372d0c353',
    help: 'Use an API token with Zone:Read and DNS:Edit access to the selected zone.',
    zoneHelp: 'Found in the domain overview in Cloudflare.',
    secretLabel: '',
  },
  porkbun: {
    name: 'Porkbun',
    tokenKey: 'dns_porkbun_api_token', secretKey: 'dns_porkbun_api_secret', sessionKey: '', zoneKey: 'dns_porkbun_zone',
    tokenLabel: 'Porkbun API Key', secretLabel: 'Porkbun Secret API Key',
    zoneLabel: 'Porkbun Zone Domain', zonePlaceholder: 'e.g. example.com',
    help: 'Create an API key and secret, and enable API access for the domain in Porkbun.',
    zoneHelp: 'The registered domain hosted by Porkbun. Leave blank to use the base domain. Minimum TTL: 600 seconds.',
  },
  digitalocean: {
    name: 'DigitalOcean',
    tokenKey: 'dns_digitalocean_api_token', secretKey: '', sessionKey: '', zoneKey: 'dns_digitalocean_zone',
    tokenLabel: 'DigitalOcean API Token', secretLabel: '',
    zoneLabel: 'DigitalOcean Zone Domain', zonePlaceholder: 'e.g. example.com',
    help: 'Use a personal access token with domain read, create, update, and delete permissions.',
    zoneHelp: 'The domain in DigitalOcean Networking. Leave blank to use the base domain. Minimum TTL: 30 seconds.',
  },
  route53: {
    name: 'Amazon Route 53',
    tokenKey: 'dns_route53_api_token', secretKey: 'dns_route53_api_secret', sessionKey: 'dns_route53_session_token', zoneKey: 'dns_route53_zone',
    tokenLabel: 'AWS Access Key ID', secretLabel: 'AWS Secret Access Key',
    zoneLabel: 'Route 53 Hosted Zone ID', zonePlaceholder: 'e.g. Z0123456789EXAMPLE',
    help: 'Allow route53:GetHostedZone, route53:ListResourceRecordSets, and route53:ChangeResourceRecordSets for the public hosted zone.',
    zoneHelp: 'The public hosted zone ID from the Route 53 console.',
  },
} as const;

export type DNSProvider = keyof typeof dnsProviders;

export function isDNSProvider(value: string): value is DNSProvider {
  return Object.hasOwn(dnsProviders, value);
}
