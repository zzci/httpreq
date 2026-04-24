import { useState, useEffect, useCallback, FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api';
import type { Domain, TXTRecord, APIKeyItem } from '../api';
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

type Tab = 'domains' | 'keys' | 'config';

export default function Dashboard() {
  const [tab, setTab] = useState<Tab>('domains');
  const [domains, setDomains] = useState<Domain[]>([]);
  const [records, setRecords] = useState<TXTRecord[]>([]);
  const [keys, setKeys] = useState<APIKeyItem[]>([]);
  const [apiDomain, setApiDomain] = useState('');
  const [username, setUsername] = useState('');
  const [newDomain, setNewDomain] = useState('');
  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyScope, setNewKeyScope] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const loadData = useCallback(async () => {
    try {
      const [doms, recs, info, profile, ks] = await Promise.all([
        api.getDomains(), api.getRecords(), api.getInfo(), api.getProfile(), api.getKeys(),
      ]);
      setDomains(doms || []);
      setRecords(recs || []);
      setApiDomain(info.api_domain);
      setUsername(profile.username);
      setKeys(ks || []);
    } catch (err: unknown) {
      if (err instanceof Error && err.message === 'invalid_token') {
        clearToken(); navigate('/login'); return;
      }
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  }, [navigate]);

  useEffect(() => { loadData(); }, [loadData]);
  useEffect(() => { document.title = 'Dashboard — httpreq'; }, []);

  const handleAddDomain = async (e: FormEvent) => {
    e.preventDefault(); setError('');
    const domain = newDomain.trim().toLowerCase();
    if (!domain) return;
    try { await api.addDomain(domain); setNewDomain(''); await loadData(); }
    catch (err: unknown) { setError(err instanceof Error ? err.message : 'Failed to add domain'); }
  };

  const handleRemoveDomain = async (domain: string) => {
    if (!confirm(`Remove ${domain}?`)) return;
    try { await api.removeDomain(domain); await loadData(); }
    catch (err: unknown) { setError(err instanceof Error ? err.message : 'Failed to remove domain'); }
  };

  const handleCreateKey = async (e: FormEvent) => {
    e.preventDefault(); setError('');
    if (!newKeyName.trim()) return;
    const scope = newKeyScope.trim()
      ? newKeyScope.split(',').map(s => s.trim().toLowerCase()).filter(Boolean)
      : ['*'];
    try { await api.createKey(newKeyName.trim(), scope); setNewKeyName(''); setNewKeyScope(''); await loadData(); }
    catch (err: unknown) { setError(err instanceof Error ? err.message : 'Failed to create key'); }
  };

  const handleDeleteKey = async (id: number, name: string) => {
    if (!confirm(`Delete key "${name}"?`)) return;
    try { await api.deleteKey(id); await loadData(); }
    catch (err: unknown) { setError(err instanceof Error ? err.message : 'Failed to delete key'); }
  };

  if (loading) return <div className="loading">Loading...</div>;

  const defaultKey = keys.find(k => k.name === 'Default' && k.scope.includes('*'));

  return (
    <div className="dashboard">
      <div className="topbar">
        <div className="topbar-left">
          <div className="topbar-logo">HTTPREQ</div>
          <div className="topbar-nav">
            <button className={`nav-item ${tab === 'domains' ? 'active' : ''}`} onClick={() => setTab('domains')}>
              Domains <span className="nav-count">{domains.length}</span>
            </button>
            <button className={`nav-item ${tab === 'keys' ? 'active' : ''}`} onClick={() => setTab('keys')}>
              API Keys <span className="nav-count">{keys.length}</span>
            </button>
            <button className={`nav-item ${tab === 'config' ? 'active' : ''}`} onClick={() => setTab('config')}>
              Config
            </button>
          </div>
        </div>
        <div className="topbar-actions">
          <span className="topbar-user">{username}</span>
          <button className="btn-ghost" onClick={() => { clearToken(); navigate('/login'); }}>Logout</button>
        </div>
      </div>

      {error && <div className="error">{error}</div>}

      {/* Domains Tab */}
      {tab === 'domains' && (
        <div className="card tab-card">
          <form className="add-form" onSubmit={handleAddDomain}>
            <input type="text" placeholder="example.com" value={newDomain}
              onChange={(e) => setNewDomain(e.target.value)} />
            <button type="submit">Add</button>
          </form>
          <table>
            <thead>
              <tr><th>Domain</th><th>CNAME Name</th><th>CNAME Value</th><th></th></tr>
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

          {records.length > 0 && (
            <>
              <div className="card-divider" />
              <div className="card-header"><span className="card-title">Active TXT Records</span></div>
              <table>
                <thead><tr><th>Domain</th><th>Value</th><th>Updated</th></tr></thead>
                <tbody>
                  {records.map((r, i) => (
                    <tr key={i}>
                      <td>{r.domain}</td>
                      <td className="mono">{r.value}</td>
                      <td>{new Date(r.last_update).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </div>
      )}

      {/* API Keys Tab */}
      {tab === 'keys' && (
        <div className="card tab-card">
          <form className="add-form" onSubmit={handleCreateKey}>
            <input type="text" placeholder="Key name" value={newKeyName}
              onChange={(e) => setNewKeyName(e.target.value)} style={{flex:'0 0 150px'}} />
            <input type="text" placeholder="Scope: *.example.com, domain.com (empty = global)"
              value={newKeyScope} onChange={(e) => setNewKeyScope(e.target.value)} />
            <button type="submit">Create</button>
          </form>
          <table>
            <thead><tr><th>Name</th><th>Key</th><th>Scope</th><th></th></tr></thead>
            <tbody>
              {keys.length === 0 ? (
                <tr><td colSpan={4} className="empty">No API keys</td></tr>
              ) : (
                keys.map((k) => (
                  <tr key={k.id}>
                    <td>{k.name}</td>
                    <td><code>{k.key}</code> <CopyBtn text={k.key} /></td>
                    <td>{k.scope.includes('*') ? <em>Global</em> : <code>{k.scope.join(', ')}</code>}</td>
                    <td><button className="btn-sm btn-delete" onClick={() => handleDeleteKey(k.id, k.name)}>Delete</button></td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* Config Tab */}
      {tab === 'config' && defaultKey && (
        <div className="card tab-card">
          <div className="config-block">
            Configure <a href="https://go-acme.github.io/lego/dns/httpreq/" target="_blank" rel="noreferrer">lego httpreq</a> provider:
            <div className="config-line"><code>HTTPREQ_ENDPOINT=https://{apiDomain}</code><CopyBtn text={`HTTPREQ_ENDPOINT=https://${apiDomain}`} /></div>
            <div className="config-line"><code>HTTPREQ_USERNAME={username}</code><CopyBtn text={`HTTPREQ_USERNAME=${username}`} /></div>
            <div className="config-line"><code>HTTPREQ_PASSWORD={defaultKey.key}</code><CopyBtn text={`HTTPREQ_PASSWORD=${defaultKey.key}`} /></div>
            <div className="config-line"><code>LEGO_DISABLE_CNAME_SUPPORT=true</code><CopyBtn text="LEGO_DISABLE_CNAME_SUPPORT=true" /></div>
          </div>
          <div className="card-divider" />
          <div className="config-block">
            <a href="https://doc.traefik.io/traefik/" target="_blank" rel="noreferrer">Traefik</a> integration:
            <div className="config-line"><code>propagation.disableChecks: true</code><CopyBtn text="propagation:\n  disableChecks: true" /></div>
          </div>
        </div>
      )}
    </div>
  );
}
