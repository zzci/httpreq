import { useState, useEffect, useCallback, FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api';
import type { Domain, TXTRecord } from '../api';
import { clearToken } from '../auth';

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <button className="btn-copy" onClick={handleCopy} title="Copy">
      {copied ? 'Copied' : 'Copy'}
    </button>
  );
}

export default function Dashboard() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [records, setRecords] = useState<TXTRecord[]>([]);
  const [baseDomain, setBaseDomain] = useState('');
  const [apiDomain, setApiDomain] = useState('');
  const [newDomain, setNewDomain] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const loadData = useCallback(async () => {
    try {
      const [doms, recs, info] = await Promise.all([
        api.getDomains(),
        api.getRecords(),
        api.getInfo(),
      ]);
      setDomains(doms || []);
      setRecords(recs || []);
      setBaseDomain(info.base_domain);
      setApiDomain(info.api_domain);
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

  useEffect(() => {
    loadData();
  }, [loadData]);

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

  const handleLogout = () => {
    clearToken();
    navigate('/login');
  };

  if (loading) return <div className="container"><p>Loading...</p></div>;

  return (
    <div className="container">
      <div className="header">
        <h1>httpdns</h1>
        <button className="btn-logout" onClick={handleLogout}>Logout</button>
      </div>

      {error && <div className="error">{error}</div>}

      <div className="section">
        <h2>My Domains</h2>
        <form className="add-form" onSubmit={handleAddDomain}>
          <input
            type="text"
            placeholder="example.com"
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
          />
          <button type="submit">Add Domain</button>
        </form>

        <table>
          <thead>
            <tr>
              <th>Domain</th>
              <th>Record Name</th>
              <th>Record Value</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {domains.length === 0 ? (
              <tr><td colSpan={4} className="empty">No domains yet</td></tr>
            ) : (
              domains.map((d) => {
                const recordName = `_acme-challenge.${d.domain}`;
                const recordValue = d.cname_target;
                return (
                  <tr key={d.domain}>
                    <td>{d.domain}</td>
                    <td>
                      <code>{recordName}</code>
                      <CopyButton text={recordName} />
                    </td>
                    <td>
                      <code>{recordValue}</code>
                      <CopyButton text={recordValue} />
                    </td>
                    <td>
                      <button className="btn-delete" onClick={() => handleRemoveDomain(d.domain)}>
                        Delete
                      </button>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      <div className="section">
        <h2>Active TXT Records</h2>
        <table>
          <thead>
            <tr>
              <th>Domain</th>
              <th>Value</th>
              <th>Last Update</th>
            </tr>
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

      <div className="section">
        <h2>httpreq Configuration</h2>
        <div className="info-box">
          Use with <a href="https://go-acme.github.io/lego/dns/httpreq/" target="_blank" rel="noreferrer">lego httpreq</a> provider:<br />
          <code>HTTPREQ_ENDPOINT=https://{apiDomain}</code>
          <CopyButton text={`HTTPREQ_ENDPOINT=https://${apiDomain}`} /><br />
          <code>HTTPREQ_USERNAME=&lt;your-username&gt;</code><br />
          <code>HTTPREQ_PASSWORD=&lt;your-password&gt;</code><br />
          <code>lego --dns httpreq --domains example.com run</code>
        </div>
      </div>
    </div>
  );
}
