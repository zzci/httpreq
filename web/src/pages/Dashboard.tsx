import { useState, useEffect, useCallback, FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api';
import type { Domain, TXTRecord } from '../api';
import { clearToken } from '../auth';

function CopyBtn({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <button className="btn-sm btn-copy" onClick={handleCopy}>
      {copied ? 'Copied!' : 'Copy'}
    </button>
  );
}

export default function Dashboard() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [records, setRecords] = useState<TXTRecord[]>([]);
  const [apiDomain, setApiDomain] = useState('');
  const [username, setUsername] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [newDomain, setNewDomain] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const loadData = useCallback(async () => {
    try {
      const [doms, recs, info, profile] = await Promise.all([
        api.getDomains(),
        api.getRecords(),
        api.getInfo(),
        api.getProfile(),
      ]);
      setDomains(doms || []);
      setRecords(recs || []);
      setApiDomain(info.api_domain);
      setUsername(profile.username);
      setApiKey(profile.api_key);
    } catch (err: unknown) {
      if (err instanceof Error && err.message === 'invalid_token') {
        clearToken();
        navigate('/login');
        return;
      }
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  }, [navigate]);

  useEffect(() => { loadData(); }, [loadData]);

  const handleAddDomain = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    const domain = newDomain.trim().toLowerCase();
    if (!domain) return;
    try {
      await api.addDomain(domain);
      setNewDomain('');
      await loadData();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to add domain');
    }
  };

  const handleRemoveDomain = async (domain: string) => {
    if (!confirm(`Remove ${domain}?`)) return;
    try {
      await api.removeDomain(domain);
      await loadData();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to remove domain');
    }
  };

  const handleRegenerateKey = async () => {
    if (!confirm('Regenerate API Key? Existing integrations will stop working.')) return;
    try {
      const res = await api.regenerateKey();
      setApiKey(res.api_key);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to regenerate key');
    }
  };

  if (loading) return <div className="loading">Loading...</div>;

  return (
    <div className="dashboard">
      <div className="topbar">
        <div className="topbar-logo">http<span>dns</span></div>
        <div className="topbar-actions">
          <span className="topbar-user">{username}</span>
          <button className="btn-ghost" onClick={() => { clearToken(); navigate('/login'); }}>
            Logout
          </button>
        </div>
      </div>

      {error && <div className="error">{error}</div>}

      {/* API Credentials */}
      <div className="card">
        <div className="card-header">
          <span className="card-title">API Credentials</span>
        </div>
        <div className="card-body">
          <div className="cred-row">
            <span className="cred-label">Username</span>
            <span className="cred-value">{username}</span>
            <CopyBtn text={username} />
          </div>
          <div className="cred-row">
            <span className="cred-label">API Key</span>
            <span className="cred-value">{apiKey}</span>
            <CopyBtn text={apiKey} />
            <button className="btn-sm btn-regen" onClick={handleRegenerateKey}>Regenerate</button>
          </div>
        </div>
      </div>

      {/* Domains */}
      <div className="card">
        <div className="card-header">
          <span className="card-title">Domains</span>
        </div>
        <form className="add-form" onSubmit={handleAddDomain}>
          <input type="text" placeholder="example.com" value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)} />
          <button type="submit">Add</button>
        </form>
        <table>
          <thead>
            <tr>
              <th>Domain</th>
              <th>CNAME Name</th>
              <th>CNAME Value</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {domains.length === 0 ? (
              <tr><td colSpan={4} className="empty">No domains added yet</td></tr>
            ) : (
              domains.map((d) => {
                const name = `_acme-challenge.${d.domain}`;
                return (
                  <tr key={d.domain}>
                    <td>{d.domain}</td>
                    <td><code>{name}</code> <CopyBtn text={name} /></td>
                    <td><code>{d.cname_target}</code> <CopyBtn text={d.cname_target} /></td>
                    <td><button className="btn-sm btn-delete" onClick={() => handleRemoveDomain(d.domain)}>Delete</button></td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* TXT Records */}
      <div className="card">
        <div className="card-header">
          <span className="card-title">Active TXT Records</span>
        </div>
        <table>
          <thead>
            <tr><th>Domain</th><th>Value</th><th>Updated</th></tr>
          </thead>
          <tbody>
            {records.length === 0 ? (
              <tr><td colSpan={3} className="empty">No active records</td></tr>
            ) : (
              records.map((r, i) => (
                <tr key={i}>
                  <td>{r.domain}</td>
                  <td className="mono">{r.value}</td>
                  <td>{new Date(r.last_update).toLocaleString()}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* httpreq Config */}
      <div className="card">
        <div className="card-header">
          <span className="card-title">httpreq Configuration</span>
        </div>
        <div className="config-block">
          Configure <a href="https://go-acme.github.io/lego/dns/httpreq/" target="_blank" rel="noreferrer">lego httpreq</a> with these environment variables:
          <div className="config-line">
            <code>HTTPREQ_ENDPOINT=https://{apiDomain}</code>
            <CopyBtn text={`HTTPREQ_ENDPOINT=https://${apiDomain}`} />
          </div>
          <div className="config-line">
            <code>HTTPREQ_USERNAME={username}</code>
            <CopyBtn text={`HTTPREQ_USERNAME=${username}`} />
          </div>
          <div className="config-line">
            <code>HTTPREQ_PASSWORD={apiKey}</code>
            <CopyBtn text={`HTTPREQ_PASSWORD=${apiKey}`} />
          </div>
          <div className="config-line">
            <code>LEGO_DISABLE_CNAME_SUPPORT=true</code>
            <CopyBtn text="LEGO_DISABLE_CNAME_SUPPORT=true" />
          </div>
        </div>
      </div>
    </div>
  );
}
