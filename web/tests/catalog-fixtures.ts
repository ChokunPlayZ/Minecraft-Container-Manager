// Provider fixtures inspect the upstream target while exercising the real proxy URL.
export function catalogTarget(input: RequestInfo | URL): string {
 const raw=String(input);const parsed=new URL(raw,'http://localhost');
 return parsed.pathname==='/api/proxy'?(parsed.searchParams.get('url')??raw):raw;
}
export function jsonResponse(value: unknown): Response {
 return new Response(JSON.stringify(value),{status:200,headers:{'Content-Type':'application/json'}});
}
export function emptyCatalog(input: RequestInfo | URL): Response {
 const target=catalogTarget(input);
 if(target.includes('api.modrinth.com/v2/search'))return jsonResponse({hits:[],offset:0,limit:20,total_hits:0});
 if(target.includes('api.modrinth.com/v2/project/'))return jsonResponse([]);
 if(target.includes('hangar.papermc.io/api/v1/projects'))return jsonResponse({pagination:{count:0,limit:20,offset:0},result:[]});
 if(target.includes('api.spiget.org/v2/'))return jsonResponse([]);
 throw new Error(`Unexpected catalog fixture request: ${target}`);
}
