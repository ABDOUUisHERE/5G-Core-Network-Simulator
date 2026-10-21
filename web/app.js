'use strict';
const $ = id => document.getElementById(id);
const state = { data: null, history: [], events: [], seen: new Map(), ready: false, page: { equipment: 0, sessions: 0 }, job: null, refreshing: false, view: 'overview' };
const views = {
 overview: ['Overview', 'Your virtual network, in real time.'],
 equipment: ['UE devices', 'Track device registration and lifecycle.'],
 sessions: ['PDU sessions', 'Manage your network connections and IP addresses.'],
 traffic: ['Virtual traffic', 'Explore latency, jitter and packet loss.'],
 observability: ['Observability', 'Measure the network and test its resilience.']
};
const number = value => new Intl.NumberFormat('en-US', { maximumFractionDigits: 1 }).format(value || 0);
const esc = value => String(value ?? '').replace(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character]);
const short = value => String(value || '').slice(0, 8);
const badge = value => `<span class="state ${esc(value)}">${esc(value)}</span>`;
const time = date => new Date(date).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
const empty = (title, description) => `<div class="empty-state"><span class="empty-icon">◇</span><strong>${esc(title)}</strong><p>${esc(description)}</p></div>`;
let toastTimer;
function toast(message, error = false) { $('toast').textContent = message; $('toast').classList.toggle('error', error); $('toast').hidden = false; clearTimeout(toastTimer); toastTimer = setTimeout(() => { $('toast').hidden = true; }, 6000); }
function event(title, description = '', error = false) { state.events.unshift({ title, description, error, at: new Date() }); state.events = state.events.slice(0, 40); renderActivity(); }
async function api(path, body) {
 const response = await fetch(`/api${path}`, { method: body === undefined ? 'GET' : 'POST', headers: body === undefined ? {} : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(12000) });
 const result = await response.json();
 if (!response.ok) throw new Error(`${result.code || response.status} · ${result.message || 'The request failed.'}`);
 return result;
}
function route() {
 const next = location.hash.slice(1); state.view = views[next] ? next : 'overview';
 document.querySelectorAll('.view').forEach(element => { element.hidden = element.id !== `view-${state.view}`; });
 document.querySelectorAll('[data-view]').forEach(link => { const active = link.dataset.view === state.view; link.classList.toggle('active', active); if (active) link.setAttribute('aria-current', 'page'); else link.removeAttribute('aria-current'); });
 $('breadcrumb').textContent = views[state.view][0]; $('page-title').textContent = views[state.view][0]; $('page-description').textContent = views[state.view][1];
 document.title = `${views[state.view][0]} — Core5G`; $('sidebar').classList.remove('open'); $('menu-button').setAttribute('aria-expanded', 'false');
}
function renderActivity() {
 $('activity').innerHTML = state.events.length ? `<div class="activity-list">${state.events.map(item => `<div class="activity-item"><span class="event-symbol ${item.error ? 'error' : ''}">${item.error ? '!' : '↗'}</span><div><strong>${esc(item.title)}</strong><p>${esc(item.description)}</p></div><span class="activity-time">${time(item.at)}</span></div>`).join('')}</div>` : empty('The lab is ready for your first steps', 'Register a device or run a simulation to observe lab activity.');
}
function spark(key, color) {
 const values = state.history.map(item => Number(item[key]) || 0); if (values.length < 2) return '';
 const max = Math.max(...values, 1), min = Math.min(...values, 0), span = max - min || 1;
 const points = values.map((value, index) => `${index / (values.length - 1) * 74},${26 - (value - min) / span * 23}`).join(' ');
 return `<svg viewBox="0 0 74 29" aria-hidden="true"><polyline fill="none" stroke="${color}" stroke-width="1.6" points="${points}"/></svg>`;
}
function kpi(label, value, unit, note, icon, key, color) { return `<article class="kpi"><div class="kpi-top"><span>${label}</span><span class="kpi-icon">${icon}</span></div><div class="kpi-value">${value}<small>${unit}</small></div><div class="kpi-bottom">${note}</div>${spark(key, color)}</article>`; }
function renderKpis() {
 const data = state.data, traffic = data.traffic || {}, sent = Number(traffic.packets_sent || 0), lost = Number(traffic.packets_lost || 0), loss = sent ? lost / sent * 100 : 0;
 const latency = state.job?.state === 'COMPLETED' ? number(state.job.p50_ms) : '—';
 $('kpis').innerHTML = kpi('Registered UEs', number(data.registered_ues), '', `${number(data.ues?.length)} devices in total`, '▣', 'registered', '#61d7b8') + kpi('Active sessions', number(data.active_sessions), '', 'Established PDU connections', '⇄', 'sessions', '#7ca9ef') + kpi('Scenario latency', latency, 'ms', state.job?.state === 'COMPLETED' ? 'Latest scenario median' : 'Measured after a simulation', '⌁', 'latency', '#bc96df') + kpi('Packets lost', number(loss), '%', `${number(lost)} lost / ${number(sent)} sent`, '↘', 'loss', '#dfb67b');
 $('traffic-kpis').innerHTML = kpi('Packets sent', number(sent), '', `${number(traffic.bytes_sent)} bytes`, '↗', 'sent', '#61d7b8') + kpi('Packets received', number(traffic.packets_received), '', `${number(traffic.bytes_received)} bytes`, '↙', 'received', '#7ca9ef') + kpi('Packets lost', number(lost), '', `${number(traffic.bytes_lost)} bytes`, '⌁', 'lost', '#bc96df') + kpi('Packets rejected', number(traffic.packets_rejected), '', `${number(traffic.bytes_rejected)} bytes`, '×', 'rejected', '#dfb67b');
 $('ue-count').textContent = number(data.ues?.length); $('session-count').textContent = number(data.active_sessions);
}
function renderTopology() {
 const services = state.data.services || [], status = name => services.find(service => service.name === name)?.status === 'UP';
 const nodes = [['ue', 'UE', `${number(state.data.registered_ues)} devices`, '▣'], ['gnb-simulator', 'gNB', 'Simulated access', '⌁'], ['amf', 'AMF', 'Registration', '◇'], ['smf', 'SMF', 'PDU sessions', '⇄'], ['upf', 'UPF', 'Virtual traffic', '⇢']];
 $('topology').innerHTML = nodes.map(([id, title, description, icon]) => `<button class="network-node ${id !== 'ue' && !status(id) ? 'down' : ''}" data-name="${id}" data-action="observe" aria-label="Inspect ${title}"><span class="node-icon">${icon}</span><strong>${title}</strong><small>${description}</small></button>`).join('') + `<div class="auth-branch">${['ausf', 'udm'].map(name => `<span class="auth-node ${status(name) ? '' : 'down'}"><span class="tiny-dot"></span>${name.toUpperCase()}</span>`).join('')}</div>`;
 const up = services.filter(service => service.status === 'UP').length;
 $('network-health').textContent = `${up} / ${services.length} services available`; $('network-health').classList.toggle('warning', up !== services.length);
 $('sidebar-state').textContent = up === services.length ? 'All services are available' : `${services.length - up} service(s) unavailable`;
 $('services-grid').innerHTML = services.map(service => `<article class="service-card"><div class="service-card-top"><h3>${esc(service.name)}</h3><span class="state ${service.status === 'UP' ? 'REGISTERED' : 'FAILED'}">${esc(service.status)}</span></div><p>Health check · ${number(service.latency_ms)} ms</p><div class="service-card-buttons"><button class="button inject" data-action="fault" data-id="${esc(service.name)}">Inject fault</button><button class="button restore" data-action="restore" data-id="${esc(service.name)}">Restore</button></div></article>`).join('');
}
function sortedUEs() { return [...(state.data?.ues || [])].sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at)); }
function ueTable(items, compact = false) {
 if (!items.length) return empty('No devices to display', 'Register a UE with a synthetic profile or run your first simulation.');
 return `<div class="table-wrap"><table><thead><tr><th>Device / SUPI</th><th>State</th>${compact ? '' : '<th>gNB</th><th>Actions</th>'}</tr></thead><tbody>${items.map(ue => `<tr><td><span class="cell-main mono">${esc(ue.supi)}</span><span class="cell-sub">UE-${short(ue.ue_id)}</span></td><td>${badge(ue.state)}</td>${compact ? '' : `<td class="mono">${esc(ue.gnb_id)}</td><td><div class="table-actions">${['REGISTERED', 'SESSION_ACTIVE'].includes(ue.state) ? `<button data-action="create-session" data-id="${ue.ue_id}">＋ Session</button><button class="danger" data-action="deregister" data-id="${ue.ue_id}">Disconnect</button>` : '—'}</div></td>`}</tr>`).join('')}</tbody></table></div>`;
}
function pagination(id, kind, count) {
 const pages = Math.max(1, Math.ceil(count / 25)); state.page[kind] = Math.min(state.page[kind], pages - 1);
 $(id).innerHTML = `<span>${number(count)} items · page ${state.page[kind] + 1} / ${pages}</span><button data-action="page-back" data-id="${kind}" ${state.page[kind] === 0 ? 'disabled' : ''}>← Previous</button><button data-action="page-next" data-id="${kind}" ${state.page[kind] >= pages - 1 ? 'disabled' : ''}>Next →</button>`;
}
function renderTables() {
 const ues = sortedUEs(), query = $('ue-search').value.trim().toLowerCase(), filtered = ues.filter(ue => `${ue.supi} ${ue.ue_id}`.toLowerCase().includes(query));
 $('recent-equipment').innerHTML = ueTable(ues.slice(0, 5), true);
 pagination('equipment-pagination', 'equipment', filtered.length);
 if (!$('equipment-table').contains(document.activeElement)) $('equipment-table').innerHTML = ueTable(filtered.slice(state.page.equipment * 25, state.page.equipment * 25 + 25));
 const sessions = [...(state.data?.sessions || [])].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
 pagination('sessions-pagination', 'sessions', sessions.length);
 const items = sessions.slice(state.page.sessions * 25, state.page.sessions * 25 + 25);
 if (!$('sessions-table').contains(document.activeElement)) $('sessions-table').innerHTML = !items.length ? empty('No PDU sessions', 'Register a UE, then create its session from the Devices page.') : `<div class="table-wrap"><table><thead><tr><th>Session / UE</th><th>Virtual IP</th><th>State</th><th>Actions</th></tr></thead><tbody>${items.map(item => `<tr><td><span class="cell-main mono">PDU-${short(item.session_id)}</span><span class="cell-sub">UE-${short(item.ue_id)}</span></td><td class="mono">${esc(item.ip_address || '—')}</td><td>${badge(item.state)}</td><td><div class="table-actions">${item.state === 'ACTIVE' ? `<button data-action="send" data-id="${item.session_id}">↗ Packet</button><button class="danger" data-action="release" data-id="${item.session_id}">Release</button>` : item.state === 'RELEASING' ? `<button data-action="release" data-id="${item.session_id}">Retry</button>` : '—'}</div></td></tr>`).join('')}</tbody></table></div>`;
 const select = $('packet-session'), old = select.value, active = sessions.filter(session => session.state === 'ACTIVE');
 if (document.activeElement !== select) { select.innerHTML = active.length ? active.map(session => `<option value="${session.session_id}">PDU-${short(session.session_id)} · ${esc(session.ip_address)}</option>`).join('') : '<option value="">No active sessions</option>'; if (active.some(session => session.session_id === old)) select.value = old; }
 $('packet-send').disabled = !active.length;
}
async function refresh() {
 if (state.refreshing) return; state.refreshing = true;
 try {
  const data = await api('/overview'); let changes = 0;
  for (const ue of data.ues || []) { if (state.ready && state.seen.get(ue.ue_id) !== ue.state && changes < 4) { event(`UE ${short(ue.ue_id)} · ${ue.state}`, ue.supi); changes++; } state.seen.set(ue.ue_id, ue.state); }
  if (!state.ready) { event('Connected to the lab', `${data.services.filter(service => service.status === 'UP').length} services available`); state.ready = true; }
  state.data = data; document.querySelectorAll('[data-tool]').forEach(link => { if(data.links?.[link.dataset.tool]) link.href=data.links[link.dataset.tool]; }); const counters = data.traffic || {}, sent = Number(counters.packets_sent || 0), lost = Number(counters.packets_lost || 0);
  state.history.push({ registered: data.registered_ues, sessions: data.active_sessions, sent, lost, received: counters.packets_received || 0, rejected: counters.packets_rejected || 0, loss: sent ? lost / sent * 100 : 0, latency: state.job?.p50_ms || 0 }); state.history = state.history.slice(-30);
  renderKpis(); renderTopology(); renderTables(); $('updated-at').textContent = `Last update · ${time(data.timestamp)}`;
  $('connection-banner').hidden = !data.warnings?.length; $('connection-banner').textContent = data.warnings?.length ? `Partial update: ${data.warnings.join(' · ')}. Check Observability for service health.` : '';
 } catch (error) { $('connection-banner').textContent = `The lab is not responding: ${error.message}`; $('connection-banner').hidden = false; $('sidebar-state').textContent = 'Connection interrupted'; }
 finally { state.refreshing = false; }
}
async function pollJob() {
 if (!state.job || state.job.state !== 'RUNNING') return;
 try { state.job = await api(`/simulations/${state.job.id}`); const job = state.job; $('job-status').textContent = `${job.completed} / ${job.count} completed · ${job.succeeded} successes · ${job.failed} failures${job.state === 'COMPLETED' ? ` · p50 ${number(job.p50_ms)} ms · p95 ${number(job.p95_ms)} ms · p99 ${number(job.p99_ms)} ms` : ''}`;
  if (job.state === 'COMPLETED') { $('simulation-start').disabled = false; $('simulation-start').innerHTML = '<span>▷</span>Run simulation'; event('Simulation completed', `${job.succeeded} / ${job.count} devices · p95 ${number(job.p95_ms)} ms`, job.failed > 0); toast(`Simulation completed: ${job.succeeded} successes, ${job.failed} failures.`, job.failed > 0); await refresh(); } else setTimeout(pollJob, 1000);
 } catch (error) { toast(error.message, true); setTimeout(pollJob, 3000); }
}
document.addEventListener('click', async click => {
 const button = click.target.closest('[data-action]'); if (!button) return;
 const action = button.dataset.action, id = button.dataset.id;
 if (action === 'observe') { location.hash = 'observability'; return; }
 if (action.startsWith('page-')) { state.page[id] += action === 'page-next' ? 1 : -1; button.blur(); renderTables(); return; }
 button.disabled = true;
 try {
  if (action === 'create-session') { const result = await api('/sessions', { ue_id: id }); toast(`Active session · ${result.ip_address}`); event('PDU session established', `UE ${short(id)} · ${result.ip_address}`); }
  if (action === 'deregister') { await api(`/ues/${id}/deregister`, {}); toast('Device disconnected.'); event('UE disconnected', short(id)); }
  if (action === 'release') { const result = await api(`/sessions/${id}/release`, {}); toast(`Session ${result.state}.`); event('Session released', short(id)); }
  if (action === 'send') { const result = await api('/packets', { session_id: id, size: 512 }); toast(result.lost ? 'Packet lost according to the simulation profile.' : `Packet received · ${number(result.latency_ms)} ms`); event(result.lost ? 'Virtual packet lost' : 'Virtual packet received', `PDU ${short(id)} · 512 bytes`); }
  if (action === 'fault' || action === 'restore') { await api(`/faults/${id}`, { enabled: action === 'fault', delay_ms: 0 }); toast(action === 'fault' ? `Fault injected into ${id}.` : `${id} restored.`); event(action === 'fault' ? 'Fault injected' : 'Service restored', id, action === 'fault'); }
  button.blur(); await refresh();
 } catch (error) { toast(error.message, true); event('Operation rejected', error.message, true); } finally { button.disabled = false; }
});
$('simulation-form').addEventListener('submit', async submit => { submit.preventDefault(); $('simulation-start').disabled = true; try { state.job = await api('/simulations', { count: Number($('simulation-count').value), concurrency: 10, full: $('simulation-full').checked }); $('job-status').hidden = false; $('job-status').textContent = 'Simulation running…'; $('simulation-start').textContent = 'Simulation running…'; event('Simulation started', `${state.job.count} devices`); pollJob(); } catch (error) { toast(error.message, true); $('simulation-start').disabled = false; } });
$('config-form').addEventListener('submit', async submit => { submit.preventDefault(); const button = submit.submitter; button.disabled = true; try { await api('/traffic/config', { latency_ms: Number($('traffic-latency').value), jitter_ms: Number($('traffic-jitter').value), loss_percent: Number($('traffic-loss').value), seed: Number($('traffic-seed').value) }); toast('Traffic profile applied.'); event('Traffic profile updated', `Seed ${$('traffic-seed').value} · loss ${$('traffic-loss').value} %`); } catch (error) { toast(error.message, true); } finally { button.disabled = false; } });
$('packet-form').addEventListener('submit', async submit => { submit.preventDefault(); $('packet-send').disabled = true; let received = 0, lost = 0; try { for (let i = 0; i < Number($('packet-count').value); i++) { const result = await api('/packets', { session_id: $('packet-session').value, size: Number($('packet-size').value), destination: $('packet-destination').value }); result.lost ? lost++ : received++; $('packet-result').textContent = `${received} received · ${lost} lost`; } event('Traffic sent', `${received} received · ${lost} lost`); await refresh(); } catch (error) { $('packet-result').textContent = error.message; toast(error.message, true); } finally { $('packet-send').disabled = !$('packet-session').value; } });
$('open-register').addEventListener('click', () => { $('register-error').textContent = ''; $('register-dialog').showModal(); });
$('close-register').addEventListener('click', () => $('register-dialog').close());
$('register-form').addEventListener('submit', async submit => { submit.preventDefault(); const button = submit.submitter; button.disabled = true; try { const input = Object.fromEntries(new FormData(submit.target)); const result = await api('/ues', input); if (!result.registration.accepted) throw new Error(result.registration.reason_code); $('register-dialog').close(); toast('Device registered.'); event('UE registered', input.supi); await refresh(); } catch (error) { $('register-error').textContent = error.message; } finally { button.disabled = false; } });
$('menu-button').addEventListener('click', () => { const open = $('sidebar').classList.toggle('open'); $('menu-button').setAttribute('aria-expanded', String(open)); });
document.addEventListener('keydown', key => { if (key.key === 'Escape') { $('sidebar').classList.remove('open'); $('menu-button').setAttribute('aria-expanded', 'false'); } });
document.querySelector('.skip').addEventListener('click', click => { click.preventDefault(); $('content').focus(); $('content').scrollIntoView(); });
$('ue-search').addEventListener('input', () => { state.page.equipment = 0; renderTables(); });
$('refresh-sessions').addEventListener('click', refresh);
document.querySelectorAll('[data-tool]').forEach(link => { const ports = { grafana: 3005, prometheus: 9095, jaeger: 16686 }; link.href = `http://${location.hostname}:${ports[link.dataset.tool]}`; });
window.addEventListener('hashchange', route); route(); renderActivity(); refresh(); setInterval(refresh, 3000);
