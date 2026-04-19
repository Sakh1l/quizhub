/* =========================================================
   QuizHub Admin Panel - admin.js
   Controls game flow. Players cannot advance on their own.
   ========================================================= */
(function () {
  'use strict';

  let adminToken = null;
  let socket = null;
  let reconnectTimer = null;

  // Game state
  let gameStatus = 'lobby'; // lobby, countdown, question, reveal, finished
  let players = [];
  let leaderboard = [];
  let questions = [];
  let answerStats = { total: 0, correct: 0, wrong: 0 };
  let currentQuestion = null;
  let questionIndex = 0;
  let totalQuestions = 0;
  let timeLeft = 0;
  let timeLimit = 15;
  let correctAnswer = null;
  let countdownLeft = 0;
  let timerInterval = null;
  let editingQuestion = null;

  const API = '';
  const $ = (sel) => document.querySelector(sel);

  function el(tag, attrs, ...children) {
    const node = document.createElement(tag);
    if (attrs) {
      Object.entries(attrs).forEach(([k, v]) => {
        if (k === 'className') node.className = v;
        else if (k.startsWith('data-')) node.setAttribute(k, v);
        else if (k === 'onclick') node.addEventListener('click', v);
        else if (k === 'onchange') node.addEventListener('change', v);
        else if (k === 'onkeydown') node.addEventListener('keydown', v);
        else if (k === 'disabled') node.disabled = v;
        else if (k === 'selected') node.selected = v;
        else node.setAttribute(k, v);
      });
    }
    children.flat().forEach(c => {
      if (c == null) return;
      node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return node;
  }

  async function api(path, opts = {}) {
    const headers = { 'Content-Type': 'application/json' };
    if (adminToken) headers['X-Admin-Token'] = adminToken;
    const res = await fetch(API + path, { headers, ...opts });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  // ---- WebSocket ----
  function connectWS() {
    if (socket && socket.readyState <= 1) return;
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(`${proto}//${location.host}/api/ws?role=admin`);

    socket.onopen = () => { clearTimeout(reconnectTimer); updateWSStatus(true); };
    socket.onmessage = (evt) => { try { handleWS(JSON.parse(evt.data)); } catch (_) {} };
    socket.onclose = () => { updateWSStatus(false); reconnectTimer = setTimeout(connectWS, 3000); };
    socket.onerror = () => socket.close();
  }

  function updateWSStatus(connected) {
    const dot = $('[data-testid="ws-status-dot"]');
    const text = $('[data-testid="ws-status-text"]');
    if (dot) dot.className = 'status-dot ' + (connected ? 'connected' : 'disconnected');
    if (text) text.textContent = connected ? 'Connected' : 'Reconnecting...';
  }

  function handleWS(msg) {
    switch (msg.event) {
      case 'player_joined':
      case 'players_update':
        if (Array.isArray(msg.data)) players = msg.data;
        refreshPlayerSection();
        break;

      case 'game_countdown':
        gameStatus = 'countdown';
        countdownLeft = msg.data.duration || 10;
        totalQuestions = msg.data.total_questions || 0;
        startCountdown();
        renderGameArea();
        break;

      case 'new_question':
        gameStatus = 'question';
        currentQuestion = msg.data.current_question;
        questionIndex = msg.data.question_index || 0;
        totalQuestions = msg.data.total_questions || 0;
        timeLeft = msg.data.time_left || 15;
        timeLimit = timeLeft;
        correctAnswer = null;
        answerStats = { total: 0, correct: 0, wrong: 0 };
        startQuestionTimer();
        renderGameArea();
        break;

      case 'time_up':
        gameStatus = 'reveal';
        correctAnswer = msg.data.correct_answer;
        clearInterval(timerInterval);
        renderGameArea();
        break;

      case 'player_answered':
        answerStats.total = msg.data.total_answers || 0;
        answerStats.correct = msg.data.correct_count || 0;
        answerStats.wrong = msg.data.wrong_count || 0;
        refreshStats();
        break;

      case 'leaderboard_update':
        leaderboard = msg.data || [];
        refreshLeaderboard();
        break;

      case 'game_finished':
        gameStatus = 'finished';
        clearInterval(timerInterval);
        renderGameArea();
        break;

      case 'game_reset':
        gameStatus = 'lobby';
        currentQuestion = null;
        players = [];
        leaderboard = [];
        answerStats = { total: 0, correct: 0, wrong: 0 };
        clearInterval(timerInterval);
        render();
        break;
    }
  }

  // ---- Timers ----
  function startCountdown() {
    clearInterval(timerInterval);
    timerInterval = setInterval(() => {
      countdownLeft -= 1;
      const cdEl = $('[data-testid="admin-countdown-number"]');
      if (cdEl) cdEl.textContent = String(Math.max(0, countdownLeft));
      if (countdownLeft <= 0) clearInterval(timerInterval);
    }, 1000);
  }

  function startQuestionTimer() {
    clearInterval(timerInterval);
    timerInterval = setInterval(() => {
      timeLeft -= 1;
      const fill = $('[data-testid="admin-timer-fill"]');
      if (fill) {
        const pct = Math.max(0, (timeLeft / timeLimit) * 100);
        fill.style.width = pct + '%';
        fill.classList.toggle('warning', timeLeft <= 5 && timeLeft > 2);
        fill.classList.toggle('critical', timeLeft <= 2);
      }
      const num = $('[data-testid="admin-timer-num"]');
      if (num) num.textContent = Math.max(0, timeLeft) + 's';
      if (timeLeft <= 0) clearInterval(timerInterval);
    }, 1000);
  }

  // ---- Render ----
  function render() {
    const app = $('#admin-app');
    app.innerHTML = '';
    if (!adminToken) { renderPIN(app); return; }
    renderDashboard(app);
  }

  function renderPIN(app) {
    const card = el('div', { className: 'card pin-screen', 'data-testid': 'pin-screen' },
      el('h2', null, 'Admin Access'),
      el('p', { className: 'subtitle' }, 'Enter the admin PIN to continue'),
      el('input', { id: 'pin-input', type: 'password', placeholder: 'Enter PIN...', 'data-testid': 'pin-input', maxlength: '20',
        onkeydown: (e) => { if (e.key === 'Enter') handlePIN(); } }),
      el('div', { style: 'margin-top:1rem' },
        el('button', { className: 'btn btn-primary', 'data-testid': 'pin-submit-btn', onclick: handlePIN, style: 'width:100%' }, 'Unlock')),
      el('p', { className: 'error-msg', 'data-testid': 'pin-error' })
    );
    app.appendChild(card);
    setTimeout(() => { const inp = $('#pin-input'); if (inp) inp.focus(); }, 50);
  }

  async function handlePIN() {
    const input = $('#pin-input');
    const errorEl = $('[data-testid="pin-error"]');
    try {
      const data = await api('/api/admin/auth', { method: 'POST', body: JSON.stringify({ pin: input.value.trim() }) });
      adminToken = data.token;
      connectWS();
      await loadInitial();
      render();
    } catch (err) { errorEl.textContent = err.message || 'Invalid PIN'; }
  }

  async function loadInitial() {
    try {
      const [p, q, s] = await Promise.all([api('/api/players'), api('/api/questions'), api('/api/game/state')]);
      players = p || []; questions = q || [];
      gameStatus = s.status || 'lobby';
      if (s.current_question) currentQuestion = s.current_question;
      questionIndex = s.question_index || 0;
      totalQuestions = s.total_questions || 0;
      if (s.correct_answer !== undefined) correctAnswer = s.correct_answer;
    } catch (_) {}
    try { leaderboard = await api('/api/leaderboard'); } catch (_) { leaderboard = []; }
  }

  function renderDashboard(app) {
    // Header
    app.appendChild(el('div', { className: 'admin-header' },
      el('div', null,
        el('h1', null, 'QuizHub Admin'),
        el('p', { className: 'subtitle' },
          el('span', { className: 'status-dot connected', 'data-testid': 'ws-status-dot' }),
          el('span', { 'data-testid': 'ws-status-text' }, 'Connected')),
      ),
      el('a', { href: '/', className: 'btn btn-secondary btn-sm', 'data-testid': 'back-to-game-btn' }, 'Player View')
    ));

    // Main layout: game area (left) + sidebar (right)
    const layout = el('div', { className: 'admin-layout' });

    // Game area
    const gameArea = el('div', { className: 'game-area', 'data-testid': 'game-area' });
    renderGameAreaInto(gameArea);
    layout.appendChild(gameArea);

    // Sidebar
    const sidebar = el('div', { className: 'sidebar' });
    sidebar.appendChild(renderLeaderboardCard());
    sidebar.appendChild(renderStatsCard());
    layout.appendChild(sidebar);

    app.appendChild(layout);

    // Bottom: Players + Questions
    const bottom = el('div', { className: 'admin-grid', style: 'margin-top:1rem' });
    bottom.appendChild(renderPlayersCard());
    bottom.appendChild(renderQuestionsCard());
    app.appendChild(bottom);
  }

  function renderGameArea() {
    const area = $('[data-testid="game-area"]');
    if (area) { area.innerHTML = ''; renderGameAreaInto(area); }
  }

  function renderGameAreaInto(container) {
    if (gameStatus === 'lobby') {
      container.appendChild(el('div', { className: 'card game-lobby', 'data-testid': 'game-lobby' },
        el('h3', null, 'Game Controls', el('span', { className: 'game-status lobby' }, 'LOBBY')),
        el('p', { style: 'color:var(--text-secondary);margin-bottom:1rem' }, `${players.length} player(s) in lobby`),
        el('div', { className: 'timer-config' },
          el('label', null, 'Question timer:'),
          el('input', { type: 'number', id: 'timer-input', value: String(timeLimit), 'data-testid': 'timer-input', style: 'width:70px;text-align:center' }),
          el('label', null, 'sec'),
          el('button', { className: 'btn btn-secondary btn-sm', onclick: handleSetTimer }, 'Set'),
        ),
        el('div', { style: 'margin-top:1rem' },
          el('button', { className: 'btn btn-accent', 'data-testid': 'admin-start-btn', onclick: handleStart,
            disabled: players.length === 0 }, 'Start Game')),
      ));

    } else if (gameStatus === 'countdown') {
      container.appendChild(el('div', { className: 'card countdown-screen', style: 'text-align:center' },
        el('h3', null, 'Game Starting!'),
        el('div', { className: 'countdown-circle' },
          el('span', { className: 'countdown-number', 'data-testid': 'admin-countdown-number' }, String(Math.max(0, countdownLeft)))),
        el('p', { style: 'color:var(--text-secondary)' }, `${totalQuestions} questions, ${players.length} players`),
      ));

    } else if (gameStatus === 'question' || gameStatus === 'reveal') {
      const q = currentQuestion;
      const isReveal = gameStatus === 'reveal';
      const optLabels = ['A', 'B', 'C', 'D', 'E', 'F'];

      const card = el('div', { className: 'card', 'data-testid': 'admin-question-card' },
        el('div', { className: 'question-meta' },
          el('span', { className: 'question-counter' }, `Question ${questionIndex + 1} of ${totalQuestions}`),
          el('span', { className: 'timer-number', 'data-testid': 'admin-timer-num' }, isReveal ? 'Time\'s up!' : (timeLeft + 's')),
        ),
        el('div', { className: 'timer-bar' },
          el('div', { className: 'timer-fill' + (isReveal ? ' critical' : ''), 'data-testid': 'admin-timer-fill',
            style: isReveal ? 'width:0%' : `width:${Math.max(0, (timeLeft / timeLimit) * 100)}%` })),
      );

      if (q) {
        card.appendChild(el('p', { className: 'question-text' }, q.text));
        card.appendChild(el('div', { className: 'options-grid' },
          ...q.options.map((opt, i) => {
            let cls = 'option-btn disabled';
            if (isReveal && i === correctAnswer) cls += ' correct';
            return el('div', { className: cls },
              el('span', { className: 'option-label' }, optLabels[i] || String(i)), opt);
          })
        ));
      }

      if (isReveal) {
        card.appendChild(el('div', { style: 'margin-top:1.25rem;text-align:right' },
          el('button', { className: 'btn btn-accent', 'data-testid': 'admin-next-btn', onclick: handleNext }, 'Next Question')));
      }

      container.appendChild(card);

    } else if (gameStatus === 'finished') {
      container.appendChild(el('div', { className: 'card', style: 'text-align:center' },
        el('h2', { style: 'margin-bottom:1rem' }, 'Game Over!'),
        el('div', { 'data-testid': 'final-leaderboard' },
          ...leaderboard.map((e, i) => {
            let rankCls = 'lb-rank';
            if (i === 0) rankCls += ' gold';
            else if (i === 1) rankCls += ' silver';
            else if (i === 2) rankCls += ' bronze';
            return el('div', { className: 'lb-entry' },
              el('span', { className: rankCls }, `#${e.rank}`),
              el('span', { className: 'lb-name' }, e.nickname),
              el('span', { className: 'lb-score' }, String(e.score)));
          })),
        el('div', { style: 'margin-top:1.5rem' },
          el('button', { className: 'btn btn-danger', 'data-testid': 'admin-reset-btn', onclick: handleReset }, 'New Game')),
      ));
    }
  }

  async function handleStart() {
    try { await api('/api/game/start', { method: 'POST' }); } catch (e) { alert(e.message); }
  }

  async function handleNext() {
    const btn = $('[data-testid="admin-next-btn"]');
    if (btn) { btn.disabled = true; btn.textContent = 'Loading...'; }
    try { await api('/api/game/next', { method: 'POST' }); } catch (e) { alert(e.message); }
  }

  async function handleReset() {
    if (!confirm('Reset game? Clears all players and scores.')) return;
    try { await api('/api/game/reset', { method: 'POST' }); } catch (_) {}
  }

  async function handleSetTimer() {
    const val = parseInt($('#timer-input')?.value);
    if (isNaN(val) || val < 5 || val > 120) return;
    try {
      await api('/api/admin/timer', { method: 'POST', body: JSON.stringify({ time_limit: val }) });
      timeLimit = val;
    } catch (_) {}
  }

  // ---- Sidebar Cards ----
  function renderLeaderboardCard() {
    const card = el('div', { className: 'card', 'data-testid': 'leaderboard-card' }, el('h3', null, 'Leaderboard'));
    const list = el('div', { 'data-testid': 'admin-leaderboard' });
    if (leaderboard.length === 0) {
      list.appendChild(el('div', { className: 'empty-state' }, 'No scores yet'));
    } else {
      leaderboard.forEach((e, i) => {
        let rankCls = 'lb-rank';
        if (i === 0) rankCls += ' gold'; else if (i === 1) rankCls += ' silver'; else if (i === 2) rankCls += ' bronze';
        list.appendChild(el('div', { className: 'lb-entry' },
          el('span', { className: rankCls }, `#${e.rank}`),
          el('span', { className: 'lb-name' }, e.nickname),
          el('span', { className: 'lb-score' }, String(e.score))));
      });
    }
    card.appendChild(list);
    return card;
  }

  function renderStatsCard() {
    return el('div', { className: 'card', 'data-testid': 'answer-stats-card' },
      el('h3', null, 'Answer Stats'),
      el('div', { className: 'answer-stats' },
        el('div', { className: 'stat-box' }, el('div', { className: 'stat-value', 'data-testid': 'stat-total' }, String(answerStats.total)), el('div', { className: 'stat-label' }, 'Answers')),
        el('div', { className: 'stat-box correct-stat' }, el('div', { className: 'stat-value', 'data-testid': 'stat-correct' }, String(answerStats.correct)), el('div', { className: 'stat-label' }, 'Correct')),
        el('div', { className: 'stat-box wrong-stat' }, el('div', { className: 'stat-value', 'data-testid': 'stat-wrong' }, String(answerStats.wrong)), el('div', { className: 'stat-label' }, 'Wrong')),
      ));
  }

  function refreshStats() {
    const t = $('[data-testid="stat-total"]'); if (t) t.textContent = String(answerStats.total);
    const c = $('[data-testid="stat-correct"]'); if (c) c.textContent = String(answerStats.correct);
    const w = $('[data-testid="stat-wrong"]'); if (w) w.textContent = String(answerStats.wrong);
  }

  function refreshLeaderboard() {
    const card = $('[data-testid="leaderboard-card"]');
    if (card) card.replaceWith(renderLeaderboardCard());
  }

  function refreshPlayerSection() {
    const card = $('[data-testid="players-card"]');
    if (card) card.replaceWith(renderPlayersCard());
  }

  // ---- Players Card ----
  function renderPlayersCard() {
    const card = el('div', { className: 'card', 'data-testid': 'players-card' },
      el('h3', null, 'Players', el('span', { className: 'badge' }, String(players.length))));
    if (players.length === 0) {
      card.appendChild(el('div', { className: 'empty-state' }, 'No players yet'));
    } else {
      const table = el('table', { className: 'player-table' },
        el('thead', null, el('tr', null, el('th', null, 'Nickname'), el('th', null, 'Score'), el('th', null, ''))),
        el('tbody', null,
          ...players.map(p => el('tr', null,
            el('td', null, p.nickname), el('td', null, String(p.score)),
            el('td', null, el('button', { className: 'kick-btn', onclick: () => handleKick(p.player_id, p.nickname) }, 'Kick'))))));
      card.appendChild(table);
    }
    return card;
  }

  async function handleKick(pid, name) {
    if (!confirm(`Kick ${name}?`)) return;
    try { await api('/api/admin/kick', { method: 'POST', body: JSON.stringify({ player_id: pid }) });
      players = players.filter(p => p.player_id !== pid); refreshPlayerSection();
    } catch (_) {}
  }

  // ---- Questions Card ----
  function renderQuestionsCard() {
    const card = el('div', { className: 'card', 'data-testid': 'questions-card' },
      el('h3', null, 'Question Bank', el('span', { className: 'badge' }, String(questions.length))),
      el('div', { className: 'btn-group', style: 'margin-bottom:1rem' },
        el('button', { className: 'btn btn-primary btn-sm', 'data-testid': 'add-question-btn', onclick: showAddModal }, '+ Add Question'),
        el('button', { className: 'btn btn-secondary btn-sm', onclick: refreshQuestions }, 'Refresh')));
    const scroll = el('div', { className: 'question-scroll' });
    questions.forEach(q => {
      const labels = ['A','B','C','D','E','F'];
      scroll.appendChild(el('div', { className: 'question-item' },
        el('div', { className: 'q-header' },
          el('div', null,
            el('div', { className: 'q-text' }, q.text),
            el('div', { className: 'q-meta' }, el('span', { className: 'q-category' }, q.category), el('span', { className: 'q-category', style: 'color:var(--accent)' }, `Answer: ${labels[q.answer]||'?'}`))),
          el('div', { className: 'q-actions' },
            el('button', { className: 'btn btn-secondary btn-sm', onclick: () => showEditModal(q) }, 'Edit'),
            el('button', { className: 'btn btn-danger btn-sm', onclick: () => handleDeleteQ(q.id) }, 'Delete')))));
    });
    card.appendChild(scroll);
    return card;
  }

  async function handleDeleteQ(id) {
    if (!confirm('Delete this question?')) return;
    try { await api('/api/questions/delete', { method: 'POST', body: JSON.stringify({ id }) });
      questions = questions.filter(q => q.id !== id); refreshQuestions();
    } catch (_) {}
  }

  async function refreshQuestions() {
    try { questions = await api('/api/questions'); } catch (_) {}
    const c = $('[data-testid="questions-card"]'); if (c) c.replaceWith(renderQuestionsCard());
  }

  function showAddModal() { editingQuestion = null; showQModal({ text: '', options: ['','','',''], answer: 0, category: '' }); }
  function showEditModal(q) { editingQuestion = q; showQModal({ ...q, options: [...q.options] }); }

  function showQModal(data) {
    const existing = $('[data-testid="question-modal"]'); if (existing) existing.remove();
    const labels = ['A','B','C','D'];
    const overlay = el('div', { className: 'modal-overlay', 'data-testid': 'question-modal',
      onclick: (e) => { if (e.target === overlay) overlay.remove(); } },
      el('div', { className: 'modal-content' },
        el('h3', null, editingQuestion ? 'Edit Question' : 'Add Question'),
        el('div', { className: 'add-question-form' },
          el('input', { id: 'q-text', type: 'text', placeholder: 'Question text...', value: data.text }),
          el('div', { className: 'options-inputs' },
            ...data.options.map((opt, i) => el('div', { className: 'option-input-wrap' },
              el('span', { className: 'opt-label' }, labels[i]||String(i)),
              el('input', { id: `q-opt-${i}`, type: 'text', placeholder: `Option ${labels[i]}`, value: opt })))),
          el('div', { className: 'form-row' },
            el('select', { id: 'q-answer' }, ...labels.map((l, i) => el('option', { value: String(i), selected: i === data.answer }, `Correct: ${l}`))),
            el('input', { id: 'q-category', type: 'text', placeholder: 'Category', value: data.category || '' })),
          el('div', { className: 'btn-group' },
            el('button', { className: 'btn btn-primary', onclick: handleSaveQ }, editingQuestion ? 'Save' : 'Add'),
            el('button', { className: 'btn btn-secondary', onclick: () => overlay.remove() }, 'Cancel')))));
    document.body.appendChild(overlay);
  }

  async function handleSaveQ() {
    const text = $('#q-text').value.trim();
    const options = [0,1,2,3].map(i => $(`#q-opt-${i}`).value.trim());
    const answer = parseInt($('#q-answer').value);
    const category = $('#q-category').value.trim() || 'general';
    if (!text || options.some(o => !o)) return;
    const overlay = $('[data-testid="question-modal"]');
    try {
      if (editingQuestion) {
        await api('/api/questions/edit', { method: 'POST', body: JSON.stringify({ id: editingQuestion.id, text, options, answer, category }) });
      } else {
        await api('/api/questions/add', { method: 'POST', body: JSON.stringify({ text, options, answer, category }) });
      }
      if (overlay) overlay.remove();
      await refreshQuestions();
    } catch (e) { alert(e.message); }
  }

  document.addEventListener('DOMContentLoaded', render);
})();
