// An OpenID Connect identity provider for the browser tests: it signs in
// whoever the test sets, without a login page, as a real provider would
// after its own sign-in: discovery, keys, authorization code with PKCE
// (S256) and RS256 ID tokens.
import { createServer } from "node:http";
import { createHash, generateKeyPairSync, randomBytes, sign } from "node:crypto";

const b64u = (b) => Buffer.from(b).toString("base64url");

export async function startIdp({ clientId = "interlinea" } = {}) {
  const { privateKey, publicKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
  const jwk = { ...publicKey.export({ format: "jwk" }), kid: "k1", alg: "RS256", use: "sig" };
  const codes = new Map();
  // Who the provider signs in next.
  let person = { sub: "eva-1", preferred_username: "eva", name: "Eva Brandt", email: "eva@agentur.example", email_verified: true };
  let issuer = "";

  const server = createServer(async (req, res) => {
    const url = new URL(req.url, issuer);
    const json = (status, body) => {
      res.writeHead(status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(body));
    };
    if (url.pathname === "/.well-known/openid-configuration")
      return json(200, {
        issuer,
        authorization_endpoint: `${issuer}/authorize`,
        token_endpoint: `${issuer}/token`,
        jwks_uri: `${issuer}/keys`,
        code_challenge_methods_supported: ["S256"],
        response_types_supported: ["code"],
        id_token_signing_alg_values_supported: ["RS256"],
      });
    if (url.pathname === "/keys") return json(200, { keys: [jwk] });
    if (url.pathname === "/authorize") {
      const q = url.searchParams;
      if (q.get("client_id") !== clientId || q.get("code_challenge_method") !== "S256") return json(400, { error: "invalid_request" });
      const code = b64u(randomBytes(16));
      codes.set(code, { challenge: q.get("code_challenge"), redirect: q.get("redirect_uri"), nonce: q.get("nonce"), person });
      const back = new URL(q.get("redirect_uri"));
      back.searchParams.set("code", code);
      back.searchParams.set("state", q.get("state"));
      res.writeHead(302, { Location: back.toString() });
      return res.end();
    }
    if (url.pathname === "/token" && req.method === "POST") {
      let body = "";
      for await (const chunk of req) body += chunk;
      const f = new URLSearchParams(body);
      const c = codes.get(f.get("code"));
      codes.delete(f.get("code"));
      const verifier = f.get("code_verifier") ?? "";
      if (!c || b64u(createHash("sha256").update(verifier).digest()) !== c.challenge || f.get("redirect_uri") !== c.redirect)
        return json(400, { error: "invalid_grant" });
      const now = Math.floor(Date.now() / 1000);
      const header = b64u(JSON.stringify({ alg: "RS256", kid: "k1", typ: "JWT" }));
      const claims = b64u(JSON.stringify({ iss: issuer, aud: clientId, iat: now, exp: now + 300, nonce: c.nonce, ...c.person }));
      const sig = b64u(sign("RSA-SHA256", Buffer.from(`${header}.${claims}`), privateKey));
      return json(200, { id_token: `${header}.${claims}.${sig}`, token_type: "Bearer", access_token: "unused" });
    }
    json(404, { error: "not_found" });
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  issuer = `http://127.0.0.1:${server.address().port}`;
  return {
    issuer,
    clientId,
    signInAs: (p) => {
      person = p;
    },
    close: () => server.close(),
  };
}
