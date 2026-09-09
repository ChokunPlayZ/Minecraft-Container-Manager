import { useState, useEffect, type FormEvent } from 'react';
import { Globe, Check, Copy, ExternalLink, RefreshCw, Trash2, AlertCircle, ShieldCheck } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { api, ApiError } from '../api/client';
import type { Server, ServerDNSResponse } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Badge } from './ui/badge';
import { useModal } from './ui/modal';

export function ServerDNSCard({
  server,
  onDnsChanged,
  bare = false,
}: {
  server: Server;
  onDnsChanged?: (joinAddress: string) => void;
  bare?: boolean;
}) {
  const [data, setData] = useState<ServerDNSResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [subdomain, setSubdomain] = useState('');
  const [targetHost, setTargetHost] = useState('');
  const [priority, setPriority] = useState(0);
  const [weight, setWeight] = useState(5);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const { confirm, dialog } = useModal();

  async function load() {
    try {
      const res = await api.getServerDNS(server.id);
      setData(res);
      if (res.record) {
        setSubdomain(res.record.subdomain ?? '');
        setTargetHost(res.record.target !== res.domain ? res.record.target : '');
        setPriority(res.record.priority ?? 0);
        setWeight(res.record.weight ?? 5);
        if (onDnsChanged && res.join_address) {
          onDnsChanged(res.join_address);
        }
      } else {
        // Suggest clean server name label if none exists
        const defaultSub = server.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
        setSubdomain(defaultSub);
      }
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.detail : 'Failed to load DNS info');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [server.id]);

  const effectiveDomain = data?.domain || 'example.com';
  const cleanSub = subdomain.trim();
  const previewJoin =
    cleanSub === '@' || cleanSub === ''
      ? effectiveDomain
      : `${cleanSub}.${effectiveDomain}`;
  const previewRecord =
    cleanSub === '@' || cleanSub === ''
      ? `_minecraft._tcp.${effectiveDomain}`
      : `_minecraft._tcp.${cleanSub}.${effectiveDomain}`;

  async function handlePublish(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setSuccess(null);
    try {
      const res = await api.publishServerDNS(server.id, {
        subdomain: subdomain.trim(),
        target: targetHost.trim() || undefined,
        port: server.host_port,
        priority,
        weight,
      });
      setSuccess('SRV record successfully published to Cloudflare!');
      if (onDnsChanged && res.join_address) {
        onDnsChanged(res.join_address);
      }
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? e.detail : 'Failed to publish DNS record');
    } finally {
      setBusy(false);
    }
  }

  async function handleRemove() {
    const ok = await confirm(
      `Remove the Cloudflare SRV record for "${server.name}"? Players will no longer be able to join via ${data?.join_address || previewJoin}.`,
      { title: 'Remove DNS Record', confirmLabel: 'Remove Record', destructive: true },
    );
    if (!ok) return;

    setBusy(true);
    setError(null);
    setSuccess(null);
    try {
      await api.removeServerDNS(server.id);
      setSuccess('SRV record removed from Cloudflare.');
      if (onDnsChanged) {
        onDnsChanged('');
      }
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? e.detail : 'Failed to remove DNS record');
    } finally {
      setBusy(false);
    }
  }

  function copyJoinAddress() {
    if (!data?.join_address) return;
    void navigator.clipboard.writeText(data.join_address);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  if (loading) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Globe className="h-4 w-4 text-primary" />
            Cloudflare SRV Routing
          </CardTitle>
          <CardDescription>Loading DNS configuration…</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  const isConfigured = data?.configured ?? false;
  const isPublished = Boolean(data?.record?.record_id);

  const bodyContent = (
    <div className="space-y-5">
      {bare && (
        <div className="flex items-center justify-between pb-1 border-b">
          <span className="text-xs font-medium text-muted-foreground">Routing Status:</span>
          {isPublished ? (
            <Badge variant="default" className="bg-emerald-600 hover:bg-emerald-500 text-white gap-1.5 py-0.5">
              <span className="h-1.5 w-1.5 rounded-full bg-white animate-pulse" />
              Active SRV
            </Badge>
          ) : isConfigured ? (
            <Badge variant="outline" className="text-muted-foreground gap-1.5 py-0.5">
              Not Published
            </Badge>
          ) : (
            <Badge variant="outline" className="border-amber-500/50 text-amber-600 dark:text-amber-400 gap-1.5 py-0.5">
              Global DNS Disabled
            </Badge>
          )}
        </div>
      )}

      {error && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}
          {success && (
            <div className="flex items-center gap-2 rounded-lg border border-emerald-500/50 bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
              <ShieldCheck className="h-4 w-4 shrink-0" />
              <span>{success}</span>
            </div>
          )}

          {!isConfigured && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4 text-sm space-y-2">
              <p className="font-medium text-amber-800 dark:text-amber-300">
                Cloudflare SRV publishing is not fully configured.
              </p>
              <p className="text-muted-foreground text-xs leading-relaxed">
                To route domain names to your Minecraft servers automatically, configure your Cloudflare API Token, Zone ID, and Domain in the global panel settings.
              </p>
              <Button asChild size="sm" variant="outline" className="mt-2 text-xs">
                <Link to="/settings" search={{ tab: 'dns' }}>
                  Open DNS Settings <ExternalLink className="ml-1.5 h-3 w-3" />
                </Link>
              </Button>
            </div>
          )}

          {isPublished && data?.join_address && (
            <div className="rounded-xl border bg-primary/5 p-4 space-y-3">
              <div className="flex items-center justify-between gap-2">
                <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Player Join Address
                </span>
                <span className="text-xs text-muted-foreground">Port {server.host_port} routed via SRV</span>
              </div>
              <div className="flex items-center justify-between gap-3 bg-background rounded-lg border p-2.5">
                <div className="flex items-center gap-2 min-w-0">
                  <Globe className="h-4 w-4 text-primary shrink-0" />
                  <span className="font-mono text-base font-semibold truncate select-all">
                    {data.join_address}
                  </span>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="gap-1.5 shrink-0 text-xs"
                  onClick={copyJoinAddress}
                >
                  {copied ? (
                    <>
                      <Check className="h-3.5 w-3.5 text-emerald-500" />
                      <span>Copied</span>
                    </>
                  ) : (
                    <>
                      <Copy className="h-3.5 w-3.5" />
                      <span>Copy</span>
                    </>
                  )}
                </Button>
              </div>
            </div>
          )}

          <form onSubmit={handlePublish} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="server-subdomain">Subdomain Label</Label>
              <div className="flex items-center gap-2">
                <Input
                  id="server-subdomain"
                  value={subdomain}
                  onChange={(e) => setSubdomain(e.target.value)}
                  placeholder="e.g. survival or @ for root"
                  className="font-mono"
                  disabled={busy}
                />
                <span className="text-sm font-medium text-muted-foreground shrink-0">
                  .{effectiveDomain}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                Enter a subdomain name (e.g. <code className="bg-muted px-1 py-0.5 rounded">play</code>) or <code className="bg-muted px-1 py-0.5 rounded">@</code> for the root apex domain ({effectiveDomain}).
              </p>
            </div>

            {/* Live DNS Preview */}
            <div className="rounded-lg border bg-muted/30 p-3 text-xs space-y-1.5">
              <div className="flex items-center justify-between text-muted-foreground font-medium">
                <span>Direct Join Address:</span>
                <span className="font-mono font-semibold text-foreground">{previewJoin}</span>
              </div>
              <div className="flex items-center justify-between text-muted-foreground font-medium">
                <span>SRV Record:</span>
                <span className="font-mono text-foreground truncate max-w-[280px] sm:max-w-none">{previewRecord}</span>
              </div>
              <div className="flex items-center justify-between text-muted-foreground font-medium">
                <span>Points to:</span>
                <span className="font-mono text-foreground">
                  {targetHost || data?.record?.target || effectiveDomain}:{server.host_port}
                </span>
              </div>
            </div>

            <div>
              <button
                type="button"
                onClick={() => setShowAdvanced(!showAdvanced)}
                className="text-xs font-medium text-primary hover:underline"
              >
                {showAdvanced ? 'Hide Advanced Options' : 'Show Advanced Options (Target Host, Priority, Weight)'}
              </button>
            </div>

            {showAdvanced && (
              <div className="space-y-3 rounded-lg border p-3 bg-muted/20">
                <div className="space-y-1.5">
                  <Label htmlFor="server-target-host" className="text-xs">
                    Target Host Override (optional)
                  </Label>
                  <Input
                    id="server-target-host"
                    value={targetHost}
                    onChange={(e) => setTargetHost(e.target.value)}
                    placeholder="e.g. mc-node1.example.com"
                    className="text-xs font-mono"
                    disabled={busy}
                  />
                  <p className="text-[11px] text-muted-foreground">
                    Defaults to the global Panel Target Host ({data?.record?.target || effectiveDomain}).
                  </p>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label htmlFor="srv-priority" className="text-xs">Priority</Label>
                    <Input
                      id="srv-priority"
                      type="number"
                      min={0}
                      value={priority}
                      onChange={(e) => setPriority(Number(e.target.value))}
                      className="text-xs"
                      disabled={busy}
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="srv-weight" className="text-xs">Weight</Label>
                    <Input
                      id="srv-weight"
                      type="number"
                      min={0}
                      value={weight}
                      onChange={(e) => setWeight(Number(e.target.value))}
                      className="text-xs"
                      disabled={busy}
                    />
                  </div>
                </div>
              </div>
            )}

            <div className="flex flex-wrap items-center gap-2 pt-2">
              <Button type="submit" disabled={busy || !isConfigured}>
                {busy ? (
                  <>
                    <RefreshCw className="h-4 w-4 mr-1.5 animate-spin" />
                    Publishing…
                  </>
                ) : isPublished ? (
                  'Update SRV Record'
                ) : (
                  'Publish SRV Record'
                )}
              </Button>

              {isPublished && (
                <Button
                  type="button"
                  variant="outline"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  disabled={busy}
                  onClick={() => void handleRemove()}
                >
                  <Trash2 className="h-4 w-4 mr-1.5" />
                  Remove SRV Record
                </Button>
              )}
            </div>
          </form>
    </div>
  );

  return (
    <>
      {dialog}
      {bare ? (
        bodyContent
      ) : (
        <Card>
          <CardHeader className="pb-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="space-y-1">
                <CardTitle className="text-base flex items-center gap-2">
                  <Globe className="h-4 w-4 text-primary" />
                  Cloudflare SRV Routing
                </CardTitle>
                <CardDescription>
                  Allow Minecraft players to join using a clean domain name without typing the port.
                </CardDescription>
              </div>
              <div>
                {isPublished ? (
                  <Badge variant="default" className="bg-emerald-600 hover:bg-emerald-500 text-white gap-1.5 py-1">
                    <span className="h-2 w-2 rounded-full bg-white animate-pulse" />
                    Active SRV
                  </Badge>
                ) : isConfigured ? (
                  <Badge variant="outline" className="text-muted-foreground gap-1.5 py-1">
                    Not Published
                  </Badge>
                ) : (
                  <Badge variant="outline" className="border-amber-500/50 text-amber-600 dark:text-amber-400 gap-1.5 py-1">
                    Global DNS Disabled
                  </Badge>
                )}
              </div>
            </div>
          </CardHeader>
          <CardContent>{bodyContent}</CardContent>
        </Card>
      )}
    </>
  );
}
