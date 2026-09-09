import { useState, useEffect, type FormEvent } from 'react';
import {
  Globe,
  Eye,
  EyeOff,
  Check,
  Trash2,
  AlertCircle,
  ShieldCheck,
  RefreshCw,
  Server as ServerIcon,
  ChevronDown,
  ChevronUp,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { DNSRecord, DNSTestResult } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Switch } from './ui/switch';
import { useModal } from './ui/modal';

export function CloudflareDNSSettingsCard() {
  const [publish, setPublish] = useState(false);
  const [domain, setDomain] = useState('');
  const [zone, setZone] = useState('');
  const [apiToken, setApiToken] = useState('');
  const [targetHost, setTargetHost] = useState('');
  const [service, setService] = useState('_minecraft');
  const [proto, setProto] = useState('_tcp');
  const [ttl, setTtl] = useState(120);
  const [priority, setPriority] = useState(0);
  const [weight, setWeight] = useState(5);

  const [hasStoredToken, setHasStoredToken] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  const [records, setRecords] = useState<DNSRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<DNSTestResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [savedMsg, setSavedMsg] = useState<string | null>(null);

  const { confirm, dialog } = useModal();

  async function load() {
    try {
      const [settingsRes, dnsRes] = await Promise.all([
        api.getSettings(),
        api.getDNS(),
      ]);

      const s = settingsRes.settings || {};
      setPublish(s.dns_publish === 'true');
      setDomain(s.dns_domain ?? '');
      setZone(s.dns_zone ?? '');
      setTargetHost(s.dns_host ?? '');
      setService(s.dns_service || '_minecraft');
      setProto(s.dns_proto || '_tcp');
      setTtl(Number(s.dns_ttl) || 120);
      setPriority(Number(s.dns_priority) || 0);
      setWeight(Number(s.dns_weight) || 5);
      setHasStoredToken(Boolean(s.dns_api_token));
      setApiToken(s.dns_api_token ?? '');

      setRecords(dnsRes.records ?? []);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.detail : 'Failed to load DNS settings');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function onSave(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    setSavedMsg(null);
    try {
      const payload: Record<string, string> = {
        dns_publish: publish ? 'true' : 'false',
        dns_domain: domain.trim(),
        dns_zone: zone.trim(),
        dns_host: targetHost.trim(),
        dns_service: service.trim() || '_minecraft',
        dns_proto: proto.trim() || '_tcp',
        dns_ttl: String(ttl || 120),
        dns_priority: String(priority),
        dns_weight: String(weight),
      };
      if (apiToken.trim()) {
        payload.dns_api_token = apiToken.trim();
      }

      await api.putSettings(payload);
      setSavedMsg('Cloudflare DNS settings saved.');
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? e.detail : 'Failed to save settings');
    } finally {
      setSaving(false);
    }
  }

  async function onTest() {
    setTesting(true);
    setTestResult(null);
    setError(null);
    try {
      const res = await api.testDNS({
        api_token: apiToken.trim() || undefined,
        zone: zone.trim() || undefined,
        domain: domain.trim() || undefined,
      });
      setTestResult(res);
    } catch (e) {
      setTestResult({
        ok: false,
        message: e instanceof ApiError ? e.detail : 'Connection test failed',
      });
    } finally {
      setTesting(false);
    }
  }

  async function removeRecord(rec: DNSRecord) {
    const ok = await confirm(
      `Remove published DNS record "${rec.name}"? Players will no longer be able to route to this server.`,
      { title: 'Remove DNS Record', confirmLabel: 'Remove', destructive: true },
    );
    if (!ok) return;

    try {
      await api.removeServerDNS(rec.server_id);
      setRecords((prev) => prev.filter((r) => r.server_id !== rec.server_id));
      setSavedMsg(`Record for ${rec.name} removed.`);
    } catch (e) {
      setError(e instanceof ApiError ? e.detail : 'Failed to remove record');
    }
  }

  if (loading) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Globe className="h-4 w-4 text-primary" />
            Cloudflare SRV Integration
          </CardTitle>
          <CardDescription>Loading Cloudflare configuration…</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  return (
    <>
      {dialog}
      <div className="space-y-6">
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-base flex items-center gap-2">
                <Globe className="h-4 w-4 text-primary" />
                Cloudflare SRV Records
              </CardTitle>
            </div>
            <CardDescription>
              Automatically publish and synchronize Minecraft SRV records using Cloudflare DNS.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSave} className="space-y-5">
              {error && (
                <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4 shrink-0" />
                  <span>{error}</span>
                </div>
              )}
              {savedMsg && (
                <div className="flex items-center gap-2 rounded-lg border border-emerald-500/50 bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
                  <ShieldCheck className="h-4 w-4 shrink-0" />
                  <span>{savedMsg}</span>
                </div>
              )}
              {testResult && (
                <div
                  className={`flex items-start gap-2 rounded-lg border p-3 text-sm ${
                    testResult.ok
                      ? 'border-emerald-500/50 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                      : 'border-destructive/50 bg-destructive/10 text-destructive'
                  }`}
                >
                  {testResult.ok ? (
                    <ShieldCheck className="h-4 w-4 shrink-0 mt-0.5" />
                  ) : (
                    <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
                  )}
                  <div className="space-y-0.5">
                    <p className="font-semibold">{testResult.ok ? 'Connection Verified' : 'Verification Failed'}</p>
                    <p className="text-xs">{testResult.message}</p>
                  </div>
                </div>
              )}

              {/* Master Toggle */}
              <div className="flex items-center justify-between rounded-xl border p-4 bg-muted/20">
                <div className="space-y-0.5 pr-4">
                  <Label htmlFor="dns-master-switch" className="text-sm font-semibold cursor-pointer">
                    Enable Cloudflare SRV Publishing
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    When enabled, MCM publishes and cleans up SRV records in your Cloudflare zone as servers start, stop, or update.
                  </p>
                </div>
                <Switch
                  id="dns-master-switch"
                  checked={publish}
                  onCheckedChange={setPublish}
                />
              </div>

              {/* Core Credentials & Config */}
              <div className="space-y-4">
                <div className="space-y-1.5">
                  <Label htmlFor="dns-api-token">Cloudflare API Token</Label>
                  <div className="relative">
                    <Input
                      id="dns-api-token"
                      type={showToken ? 'text' : 'password'}
                      value={apiToken}
                      onChange={(e) => setApiToken(e.target.value)}
                      placeholder={hasStoredToken ? '••••••••••••••••••••••••' : 'Cloudflare API Token'}
                      autoComplete="off"
                      className="pr-10 font-mono text-sm"
                    />
                    <button
                      type="button"
                      onClick={() => setShowToken(!showToken)}
                      className="absolute right-3 top-2.5 text-muted-foreground hover:text-foreground"
                      aria-label={showToken ? 'Hide token' : 'Show token'}
                    >
                      {showToken ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Create an API token in Cloudflare with <span className="font-semibold">Zone.DNS:Edit</span> permissions for your zone.
                  </p>
                </div>

                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <Label htmlFor="dns-zone">Cloudflare Zone ID</Label>
                    <Input
                      id="dns-zone"
                      value={zone}
                      onChange={(e) => setZone(e.target.value)}
                      placeholder="e.g. 023e105f4ecef8ad9ca31a8372d0c353"
                      className="font-mono text-sm"
                    />
                    <p className="text-xs text-muted-foreground">
                      Found in the right sidebar of your domain overview on Cloudflare.
                    </p>
                  </div>

                  <div className="space-y-1.5">
                    <Label htmlFor="dns-domain">Base Domain</Label>
                    <Input
                      id="dns-domain"
                      value={domain}
                      onChange={(e) => setDomain(e.target.value)}
                      placeholder="e.g. example.com"
                      className="font-mono text-sm"
                    />
                    <p className="text-xs text-muted-foreground">
                      The root domain or subdomain managed by this zone.
                    </p>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="dns-host">Default Target Host / Node</Label>
                  <Input
                    id="dns-host"
                    value={targetHost}
                    onChange={(e) => setTargetHost(e.target.value)}
                    placeholder="e.g. node1.example.com or play.example.com"
                    className="font-mono text-sm"
                  />
                  <p className="text-xs text-muted-foreground">
                    The public hostname or A-record that Minecraft SRV records point towards.
                  </p>
                </div>
              </div>

              {/* Advanced Settings Toggle */}
              <div>
                <button
                  type="button"
                  onClick={() => setShowAdvanced(!showAdvanced)}
                  className="inline-flex items-center gap-1.5 text-xs font-medium text-primary hover:underline"
                >
                  {showAdvanced ? (
                    <>
                      <ChevronUp className="h-3.5 w-3.5" /> Hide Advanced DNS Settings
                    </>
                  ) : (
                    <>
                      <ChevronDown className="h-3.5 w-3.5" /> Show Advanced DNS Settings (Service, Protocol, TTL, Defaults)
                    </>
                  )}
                </button>
              </div>

              {showAdvanced && (
                <div className="space-y-4 rounded-xl border p-4 bg-muted/20">
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                    <div className="space-y-1.5">
                      <Label htmlFor="dns-service" className="text-xs">Service</Label>
                      <Input
                        id="dns-service"
                        value={service}
                        onChange={(e) => setService(e.target.value)}
                        placeholder="_minecraft"
                        className="font-mono text-xs"
                      />
                      <p className="text-[11px] text-muted-foreground">Default: _minecraft</p>
                    </div>

                    <div className="space-y-1.5">
                      <Label htmlFor="dns-proto" className="text-xs">Protocol</Label>
                      <Input
                        id="dns-proto"
                        value={proto}
                        onChange={(e) => setProto(e.target.value)}
                        placeholder="_tcp"
                        className="font-mono text-xs"
                      />
                      <p className="text-[11px] text-muted-foreground">Default: _tcp</p>
                    </div>

                    <div className="space-y-1.5">
                      <Label htmlFor="dns-ttl" className="text-xs">TTL (seconds)</Label>
                      <Input
                        id="dns-ttl"
                        type="number"
                        min={1}
                        value={ttl}
                        onChange={(e) => setTtl(Number(e.target.value))}
                        className="font-mono text-xs"
                      />
                      <p className="text-[11px] text-muted-foreground">1 for Auto, or 120+</p>
                    </div>
                  </div>

                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <div className="space-y-1.5">
                      <Label htmlFor="dns-default-priority" className="text-xs">Default Priority</Label>
                      <Input
                        id="dns-default-priority"
                        type="number"
                        min={0}
                        value={priority}
                        onChange={(e) => setPriority(Number(e.target.value))}
                        className="text-xs"
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="dns-default-weight" className="text-xs">Default Weight</Label>
                      <Input
                        id="dns-default-weight"
                        type="number"
                        min={0}
                        value={weight}
                        onChange={(e) => setWeight(Number(e.target.value))}
                        className="text-xs"
                      />
                    </div>
                  </div>
                </div>
              )}

              <div className="flex flex-wrap items-center gap-3 pt-2">
                <Button type="submit" disabled={saving}>
                  {saving ? (
                    <>
                      <RefreshCw className="h-4 w-4 mr-1.5 animate-spin" />
                      Saving…
                    </>
                  ) : (
                    'Save DNS Settings'
                  )}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void onTest()}
                  disabled={testing || (!apiToken && !hasStoredToken) || !zone}
                >
                  {testing ? (
                    <>
                      <RefreshCw className="h-4 w-4 mr-1.5 animate-spin" />
                      Verifying…
                    </>
                  ) : (
                    <>
                      <ShieldCheck className="h-4 w-4 mr-1.5 text-primary" />
                      Test Cloudflare Connection
                    </>
                  )}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>

        {/* Active Published Records Table */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-base flex items-center gap-2">
                <ServerIcon className="h-4 w-4 text-primary" />
                Active Published SRV Records
              </CardTitle>
              <span className="text-xs text-muted-foreground font-mono">
                {records.length} {records.length === 1 ? 'record' : 'records'}
              </span>
            </div>
            <CardDescription>
              Currently registered SRV records managed by Minecraft Container Manager.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {records.length === 0 ? (
              <p className="text-sm text-muted-foreground py-2">
                No active SRV records published. Start a server or publish one from the server settings.
              </p>
            ) : (
              <div className="overflow-x-auto rounded-lg border">
                <table className="w-full text-left text-sm">
                  <thead className="border-b bg-muted/40 text-xs font-semibold uppercase text-muted-foreground">
                    <tr>
                      <th className="px-4 py-2.5">Join Address / Subdomain</th>
                      <th className="px-4 py-2.5">SRV Record Name</th>
                      <th className="px-4 py-2.5">Target & Port</th>
                      <th className="px-4 py-2.5">Priority / Weight</th>
                      <th className="px-4 py-2.5 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {records.map((r) => {
                      const joinAddr =
                        r.subdomain && r.subdomain !== '@'
                          ? `${r.subdomain}.${domain || 'domain'}`
                          : domain || 'apex';
                      return (
                        <tr key={r.server_id} className="hover:bg-muted/20 transition-colors">
                          <td className="px-4 py-3 font-medium">
                            <div className="flex items-center gap-2">
                              <Globe className="h-3.5 w-3.5 text-primary shrink-0" />
                              <span className="font-mono text-xs">{joinAddr}</span>
                            </div>
                          </td>
                          <td className="px-4 py-3 font-mono text-xs text-muted-foreground truncate max-w-[220px]">
                            {r.name}
                          </td>
                          <td className="px-4 py-3 font-mono text-xs">
                            {r.target}:{r.port}
                          </td>
                          <td className="px-4 py-3 text-xs text-muted-foreground">
                            {r.priority} / {r.weight}
                          </td>
                          <td className="px-4 py-3 text-right">
                            <Button
                              size="sm"
                              variant="ghost"
                              className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                              aria-label={`Remove record ${r.name}`}
                              onClick={() => void removeRecord(r)}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}
