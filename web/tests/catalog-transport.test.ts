import { getProjectVersions } from '../src/api/modrinth';
import { api } from '../src/api/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearRateLimitCache, rateLimitedFetchJson } from '../src/api/rate-limited-fetch';

beforeEach(()=>{vi.useFakeTimers();clearRateLimitCache();document.cookie='mcm_csrf=catalog-token; path=/';});
afterEach(()=>{vi.useRealTimers();vi.unstubAllGlobals();clearRateLimitCache();});

describe('production catalog transport',()=>{
 it('sends POSTs through the authenticated proxy with CSRF',async()=>{
  const fetch=vi.fn(async()=>Response.json({ok:true}));vi.stubGlobal('fetch',fetch);
  await rateLimitedFetchJson('https://api.modrinth.com/v2/version_files',{method:'POST',body:'{"hashes":[]}'});
  expect(fetch).toHaveBeenCalledOnce();const [url,init]=fetch.mock.calls[0] as unknown as [string,RequestInit];
  expect(url).toMatch(/^\/api\/proxy\?url=/);expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('catalog-token');expect(init.credentials).toBe('include');
 });
 it('reserves paced slots before waiting and respects actual domain concurrency',async()=>{
  let active=0,max=0;const dispatched:number[]=[];
  vi.stubGlobal('fetch',vi.fn(async()=>{active++;max=Math.max(max,active);dispatched.push(Date.now());await new Promise(r=>setTimeout(r,10));active--;return Response.json({ok:true});}));
  const prime=rateLimitedFetchJson('https://api.spiget.org/v2/resources?prime=1');await vi.runAllTimersAsync();await prime;
  const pending=Array.from({length:8},(_,i)=>rateLimitedFetchJson(`https://api.spiget.org/v2/resources?n=${i}`));
  await vi.runAllTimersAsync();await Promise.all(pending);
  expect(max).toBe(1);for(let i=1;i<dispatched.length;i++)expect(dispatched[i]-dispatched[i-1]).toBeGreaterThanOrEqual(300);
 });
 it('drains past an aborted queued request',async()=>{
  const fetch=vi.fn(async()=>{await new Promise(r=>setTimeout(r,100));return Response.json({ok:true});});vi.stubGlobal('fetch',fetch);
  const abort=new AbortController();const first=rateLimitedFetchJson('https://api.spiget.org/v2/resources?first=1');
  const cancelled=rateLimitedFetchJson('https://api.spiget.org/v2/resources?second=1',undefined,{signal:abort.signal}).catch(error=>error);
  const third=rateLimitedFetchJson('https://api.spiget.org/v2/resources?third=1');abort.abort();await vi.runAllTimersAsync();await first;await third;
  expect(await cancelled).toMatchObject({name:'AbortError'});expect(fetch).toHaveBeenCalledTimes(2);
 });
 it('never reuses cached or in-flight results from another API key',async()=>{
  const fetch=vi.fn(async(_url:string,init?:RequestInit)=>Response.json({key:new Headers(init?.headers).get('x-api-key')}));vi.stubGlobal('fetch',fetch);
  const url='https://api.curseforge.com/v1/mods/search?gameId=432';
  const a=rateLimitedFetchJson(url,{headers:{'x-api-key':'A'}});await vi.runAllTimersAsync();expect(await a).toEqual({key:'A'});
  const b=rateLimitedFetchJson(url,{headers:{'x-api-key':'B'}});await vi.runAllTimersAsync();expect(await b).toEqual({key:'B'});
  expect(fetch).toHaveBeenCalledTimes(2);
 });
 it('evicts expired responses instead of serving them forever on failures',async()=>{
  const fetch=vi.fn().mockResolvedValueOnce(Response.json({version:1})).mockResolvedValueOnce(new Response('',{status:429}));vi.stubGlobal('fetch',fetch);
  const url='https://api.modrinth.com/v2/project/test';await rateLimitedFetchJson(url,undefined,{cacheTtlMs:100});await vi.advanceTimersByTimeAsync(200);
  await expect(rateLimitedFetchJson(url)).rejects.toThrow(/Rate limit/);
 });
 it('carries declared catalog checksums into the artifact publication request',async()=>{
  const artifact='https://cdn.modrinth.com/data/test/version/mod.jar';const digest='a'.repeat(64);
  const fetch=vi.fn(async(url:string)=>url.startsWith('/api/proxy')
   ?Response.json([{id:'version',files:[{url:artifact,filename:'mod.jar',hashes:{sha256:digest}}]}])
   :Response.json({name:'mod',file:'mod.jar',enabled:true}));vi.stubGlobal('fetch',fetch);
  await getProjectVersions('test');await api.downloadMod('server',artifact,'mod.jar');
  const calls=fetch.mock.calls as unknown as [string,RequestInit][];
  expect(JSON.parse(String(calls[calls.length-1][1].body)).hashes).toEqual({sha256:digest});
 });

});
