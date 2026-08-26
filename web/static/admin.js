/* =========================================================
   QuizHub Admin Panel - admin.js
   Flow: PIN → Quiz Library → Quiz Editor → Room → Lobby → Game
   ========================================================= */
(function () {
  'use strict';

  let adminToken = null;
  let socket = null;
  let reconnectTimer = null;

  let adminStep = 'pin'; // pin, library, editor, room, countdown, question, reveal, finished
  let players = [];
  let leaderboard = [];
  let quizzes = [];
  let currentQuiz = null; // { id, title, question_count } while in the editor
  let questions = []; // questions belonging to currentQuiz
  let answerStats = { total: 0, correct: 0, wrong: 0 };
  let currentQuestion = null;
  let questionIndex = 0;
  let totalQuestions = 0;
  let timeLeft = 0;
  let timeLimit = 15;
  let correctAnswer = null;
  let countdownLeft = 0;
  let timerInterval = null;
  let roomCode = '';
  let roomLink = '';
  let adminInFlight = false;
  let toastTimer = null;

  const API = '';
  const $ = (sel) => document.querySelector(sel);

  function notify(message, tone = 'neutral') {
    let toast = $('[data-testid="admin-toast"]');
    if (!toast) {
      toast = el('div', { className: 'toast', 'data-testid': 'admin-toast', role: 'status', 'aria-live': 'polite' });
      document.body.appendChild(toast);
    }
    toast.className = `toast ${tone}`;
    toast.textContent = message;
    toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { toast.hidden = true; }, 3600);
  }

  function el(tag, attrs, ...children) {
    const node = document.createElement(tag);
    if (attrs) {
      Object.entries(attrs).forEach(([k, v]) => {
        if (k === 'className') node.className = v;
        else if (k.startsWith('data-')) node.setAttribute(k, v);
        else if (k === 'onclick') node.addEventListener('click', v);
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
    const headers = { 'Content-Type': 'application/json', ...(opts.headers || {}) };
    if (adminToken) headers['X-Admin-Token'] = adminToken;
    const res = await fetch(API + path, { headers, ...opts });
    const raw = await res.text();
    let data = {};
    try { data = raw ? JSON.parse(raw) : {}; } catch (_) { data = { error: raw || 'Request failed' }; }
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  // ---- WebSocket ----
  function connectWS() {
    if (!adminToken || (socket && socket.readyState <= 1)) return;
    clearTimeout(reconnectTimer);
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(`${proto}//${location.host}/api/ws?role=admin&admin_token=${encodeURIComponent(adminToken)}`);
    socket.onopen = () => { clearTimeout(reconnectTimer); updateWSStatus(true); };
    socket.onmessage = (evt) => { try { handleWS(JSON.parse(evt.data)); } catch (_) {} };
    socket.onclose = () => {
      socket = null;
      updateWSStatus(false);
      if (adminToken && !reconnectTimer) reconnectTimer = setTimeout(() => { reconnectTimer = null; connectWS(); }, 3000);
    };
    socket.onerror = () => { try { socket.close(); } catch (_) {} };
  }

  function updateWSStatus(connected) {
    const dot = $('[data-testid="ws-status-dot"]');
    const text = $('[data-testid="ws-status-text"]');
    if (dot) dot.className = 'status-dot ' + (connected ? 'connected' : 'disconnected');
    if (text) text.textContent = connected ? 'Live' : 'Reconnecting...';
  }

  function handleWS(msg) {
    switch (msg.event) {
      case 'player_joined':
      case 'players_update':
        if (Array.isArray(msg.data)) players = msg.data;
        refreshPlayerSection();
        break;
      case 'game_countdown':
        adminStep = 'countdown';
        countdownLeft = msg.data.duration || 10;
        totalQuestions = msg.data.total_questions || 0;
        startCountdown();
        render();
        break;
      case 'new_question':
        adminStep = 'question';
        currentQuestion = msg.data.current_question;
        questionIndex = msg.data.question_index || 0;
        totalQuestions = msg.data.total_questions || 0;
        timeLeft = msg.data.time_left || 15;
        timeLimit = timeLeft;
        correctAnswer = null;
        answerStats = { total: 0, correct: 0, wrong: 0 };
        startQuestionTimer();
        render();
        break;
      case 'time_up':
        adminStep = 'reveal';
        correctAnswer = msg.data.correct_answer;
        clearInterval(timerInterval);
        render();
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
        adminStep = 'finished';
        clearInterval(timerInterval);
        render();
        break;
      case 'game_reset':
        backToLibrary();
        break;
    }
  }

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

    if (adminStep === 'pin') { renderPIN(app); return; }

    // Header (shown for all admin steps after login)
    app.appendChild(el('div', { className: 'admin-header' },
      el('div', null,
        el('h1', null, 'QuizHub Admin'),
        el('p', { className: 'subtitle' },
          el('span', { className: 'status-dot connected', 'data-testid': 'ws-status-dot' }),
          el('span', { 'data-testid': 'ws-status-text' }, 'Live')),
      ),
      el('div', { style: 'display:flex;gap:0.6rem;align-items:center;flex-wrap:wrap;justify-content:flex-end' },
        roomCode ? el('div', { className: 'room-badge', 'data-testid': 'room-badge' }, `Room: ${roomCode}`) : null,
        el('button', { className: 'btn btn-danger btn-sm', 'data-testid': 'reset-session-btn', onclick: handleResetSession }, 'Reset Session')
      ),
    ));

    if (adminStep === 'library') renderLibrary(app);
    else if (adminStep === 'editor') renderEditor(app);
    else if (adminStep === 'room') renderRoomLobby(app);
    else if (adminStep === 'countdown') renderCountdownView(app);
    else if (adminStep === 'question' || adminStep === 'reveal') renderGameView(app);
    else if (adminStep === 'finished') renderFinishedView(app);
  }

  // ---- PIN Screen ----
  function renderPIN(app) {
    const card = el('div', { className: 'card pin-screen', 'data-testid': 'pin-screen' },
      el('h2', null, 'Admin Access'),
      el('p', { className: 'subtitle' }, 'Enter the admin PIN'),
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
    if (adminInFlight) return;
    const errorEl = $('[data-testid="pin-error"]');
    const btn = $('[data-testid="pin-submit-btn"]');
    const pin = $('#pin-input')?.value.trim();
    if (!pin) { errorEl.textContent = 'Enter the admin PIN to continue.'; return; }
    adminInFlight = true;
    btn.disabled = true;
    btn.textContent = 'Unlocking…';
    errorEl.textContent = '';
    try {
      const data = await api('/api/admin/auth', { method: 'POST', body: JSON.stringify({ pin }) });
      adminToken = data.token;
      connectWS();
      await loadLibrary();
    } catch (err) {
      errorEl.textContent = err.message || 'Invalid PIN';
      btn.disabled = false;
      btn.textContent = 'Unlock';
    } finally { adminInFlight = false; }
  }

  // ---- Library: saved, reusable quizzes ----
  async function loadLibrary() {
    try { quizzes = await api('/api/admin/quizzes'); } catch (_) { quizzes = []; }
    adminStep = 'library';
    render();
  }

  function backToLibrary() {
    currentQuiz = null;
    questions = [];
    players = [];
    leaderboard = [];
    roomCode = '';
    roomLink = '';
    answerStats = { total: 0, correct: 0, wrong: 0 };
    correctAnswer = null;
    currentQuestion = null;
    questionIndex = 0;
    totalQuestions = 0;
    timeLeft = 0;
    countdownLeft = 0;
    clearInterval(timerInterval);
    loadLibrary();
  }

  function renderLibrary(app) {
    const card = el('div', { className: 'card', 'data-testid': 'library-screen' },
      el('h2', { style: 'margin-bottom:0.25rem' }, 'Your Quizzes'),
      el('p', { className: 'subtitle', style: 'margin-bottom:1.5rem' }, 'Create a reusable quiz, add questions once, then host it any time'),

      el('div', { className: 'add-q-form', 'data-testid': 'new-quiz-form' },
        el('div', { className: 'form-row' },
          el('input', { id: 'new-quiz-title', type: 'text', placeholder: 'New quiz title...', 'data-testid': 'new-quiz-title-input',
            onkeydown: (e) => { if (e.key === 'Enter') handleCreateQuiz(); } }),
          el('button', { className: 'btn btn-primary', 'data-testid': 'create-quiz-btn', onclick: handleCreateQuiz }, 'New Quiz'),
        ),
        el('p', { className: 'error-msg', 'data-testid': 'create-quiz-error' }),
      ),

      el('div', { style: 'margin-top:1.5rem' },
        quizzes.length === 0
          ? el('p', { className: 'empty-state' }, 'No quizzes yet — create one above to get started')
          : el('div', { className: 'quiz-grid', 'data-testid': 'quiz-list' }, ...quizzes.map(renderQuizCard)),
      ),
    );
    app.appendChild(card);
  }

  function renderQuizCard(q) {
    return el('div', { className: 'quiz-card' },
      el('div', { className: 'quiz-card-title' }, q.title),
      el('div', { className: 'quiz-card-meta' }, `${q.question_count} question${q.question_count === 1 ? '' : 's'}`),
      el('div', { className: 'quiz-card-actions' },
        el('button', { className: 'btn btn-primary btn-sm', onclick: () => openEditor(q) }, 'Edit'),
        el('button', { className: 'btn btn-secondary btn-sm', onclick: () => handleDuplicateQuiz(q.id) }, 'Duplicate'),
        el('button', { className: 'btn btn-danger btn-sm', onclick: () => handleDeleteQuiz(q.id, q.title) }, 'Delete'),
      ),
    );
  }

  async function handleCreateQuiz() {
    const title = $('#new-quiz-title')?.value.trim();
    const errorEl = $('[data-testid="create-quiz-error"]');
    if (!title) { errorEl.textContent = 'Enter a title for the quiz'; return; }
    errorEl.textContent = '';
    try {
      const quiz = await api('/api/admin/quizzes', { method: 'POST', body: JSON.stringify({ title }) });
      await openEditor(quiz);
    } catch (err) { errorEl.textContent = err.message; }
  }

  async function handleDuplicateQuiz(id) {
    try {
      await api('/api/admin/quizzes/duplicate', { method: 'POST', body: JSON.stringify({ id }) });
      await loadLibrary();
    } catch (err) { notify(err.message || 'Could not duplicate quiz.', 'error'); }
  }

  async function handleDeleteQuiz(id, title) {
    if (!window.confirm(`Delete "${title}" and all of its questions? This cannot be undone.`)) return;
    try {
      await api('/api/admin/quizzes/delete', { method: 'POST', body: JSON.stringify({ id }) });
      await loadLibrary();
    } catch (err) { notify(err.message || 'Could not delete quiz.', 'error'); }
  }

  // ---- Editor: add/remove questions for one quiz ----
  async function openEditor(quiz) {
    currentQuiz = quiz;
    try { questions = await api(`/api/admin/quizzes/questions?quiz_id=${quiz.id}`); } catch (_) { questions = []; }
    adminStep = 'editor';
    render();
  }

  function renderEditor(app) {
    const card = el('div', { className: 'card', 'data-testid': 'editor-screen' },
      el('div', { style: 'display:flex;justify-content:space-between;align-items:center;margin-bottom:0.25rem;flex-wrap:wrap;gap:0.5rem' },
        el('h2', null, currentQuiz.title),
        el('button', { className: 'btn btn-secondary btn-sm', 'data-testid': 'back-to-library-btn', onclick: () => loadLibrary() }, '← Back to Library'),
      ),
      el('p', { className: 'subtitle', style: 'margin-bottom:1.5rem' }, 'Add questions, then host this quiz to share with players'),

      // Rename
      el('div', { className: 'form-row', style: 'margin-bottom:1.5rem' },
        el('input', { id: 'rename-quiz-title', type: 'text', value: currentQuiz.title, 'data-testid': 'rename-quiz-input' }),
        el('button', { className: 'btn btn-secondary btn-sm', onclick: handleRenameQuiz }, 'Rename'),
      ),

      // Add question form
      el('div', { className: 'add-q-form', 'data-testid': 'add-q-form' },
        el('input', { id: 'q-text', type: 'text', placeholder: 'Question text...', 'data-testid': 'q-text-input' }),
        el('div', { className: 'options-inputs' },
          ...['A', 'B', 'C', 'D'].map((l, i) =>
            el('div', { className: 'option-input-wrap' },
              el('span', { className: 'opt-label' }, l),
              el('input', { id: `q-opt-${i}`, type: 'text', placeholder: `Option ${l}`, 'data-testid': `q-opt-${i}` }))
          )
        ),
        el('div', { className: 'form-row' },
          el('select', { id: 'q-answer', 'data-testid': 'q-answer-select' },
            ...['A', 'B', 'C', 'D'].map((l, i) => el('option', { value: String(i) }, `Correct: ${l}`))),
          el('button', { className: 'btn btn-primary', 'data-testid': 'add-q-btn', onclick: handleAddQuestion }, 'Add Question'),
        ),
        el('p', { className: 'error-msg', 'data-testid': 'add-q-error' }),
      ),

      // Question list
      el('div', { style: 'margin-top:1.5rem' },
        el('h3', null, `Questions (${questions.length})`, questions.length === 0 ? el('span', { style: 'color:var(--text-muted);font-weight:400;font-size:0.85rem;margin-left:0.5rem' }, 'Add at least one') : null),
        el('div', { className: 'question-scroll', 'data-testid': 'question-list' },
          ...questions.map((q, idx) => {
            const labels = ['A','B','C','D'];
            return el('div', { className: 'question-item' },
              el('div', { className: 'q-header' },
                el('div', null,
                  el('div', { className: 'q-text' }, `${idx+1}. ${q.text}`),
                  el('div', { className: 'q-meta' }, el('span', { className: 'q-category', style: 'color:var(--accent)' }, `Answer: ${labels[q.answer]||'?'}`))),
                el('button', { className: 'btn btn-danger btn-sm', onclick: () => handleDeleteQ(q.id) }, 'Remove'),
              ));
          })
        ),
      ),

      // Timer config + Host button
      el('div', { style: 'margin-top:1.5rem;display:flex;gap:0.75rem;align-items:center;flex-wrap:wrap' },
        el('label', { style: 'font-size:0.85rem;color:var(--text-secondary)' }, 'Timer per question:'),
        el('input', { type: 'number', id: 'timer-input', value: String(timeLimit), style: 'width:70px;text-align:center', 'data-testid': 'timer-input' }),
        el('span', { style: 'font-size:0.85rem;color:var(--text-secondary)' }, 'seconds'),
        el('div', { style: 'flex:1' }),
        el('button', { className: 'btn btn-accent', 'data-testid': 'create-room-btn', onclick: handleCreateRoom, disabled: questions.length === 0 }, 'Host This Quiz'),
      ),
      el('p', { className: 'error-msg', 'data-testid': 'host-error' }),
    );
    app.appendChild(card);
  }

  async function handleRenameQuiz() {
    const title = $('#rename-quiz-title')?.value.trim();
    if (!title || title === currentQuiz.title) return;
    try {
      await api('/api/admin/quizzes/rename', { method: 'POST', body: JSON.stringify({ id: currentQuiz.id, title }) });
      currentQuiz.title = title;
      render();
    } catch (err) { notify(err.message || 'Could not rename quiz.', 'error'); }
  }

  async function handleAddQuestion() {
    const text = $('#q-text')?.value.trim();
    const options = [0,1,2,3].map(i => $(`#q-opt-${i}`)?.value.trim());
    const answer = parseInt($('#q-answer')?.value);
    const errorEl = $('[data-testid="add-q-error"]');

    if (!text) { errorEl.textContent = 'Enter question text'; return; }
    if (options.some(o => !o)) { errorEl.textContent = 'Fill all 4 options'; return; }

    errorEl.textContent = '';
    try {
      await api('/api/admin/quizzes/questions', { method: 'POST', body: JSON.stringify({ quiz_id: currentQuiz.id, text, options, answer, category: 'custom' }) });
      questions = await api(`/api/admin/quizzes/questions?quiz_id=${currentQuiz.id}`);
      currentQuiz.question_count = questions.length;
      // Clear form
      if ($('#q-text')) $('#q-text').value = '';
      [0,1,2,3].forEach(i => { if ($(`#q-opt-${i}`)) $(`#q-opt-${i}`).value = ''; });
      render();
    } catch (err) { errorEl.textContent = err.message; }
  }

  async function handleDeleteQ(id) {
    try {
      await api('/api/questions/delete', { method: 'POST', body: JSON.stringify({ id }) });
      questions = questions.filter(q => q.id !== id);
      currentQuiz.question_count = questions.length;
      render();
    } catch (_) {}
  }

  async function handleCreateRoom() {
    if (adminInFlight) return;
    const errorEl = $('[data-testid="host-error"]');
    const btn = $('[data-testid="create-room-btn"]');
    if (errorEl) errorEl.textContent = '';
    adminInFlight = true;
    if (btn) { btn.disabled = true; btn.textContent = 'Preparing room…'; }

    // Set timer first
    const timerVal = parseInt($('#timer-input')?.value);
    if (timerVal >= 5 && timerVal <= 120) {
      try { await api('/api/admin/timer', { method: 'POST', body: JSON.stringify({ time_limit: timerVal }) }); timeLimit = timerVal; } catch (_) {}
    }

    try {
      const data = await api('/api/room/create', { method: 'POST', body: JSON.stringify({ quiz_id: currentQuiz.id }) });
      roomCode = data.room_code;
      roomLink = data.link;
      adminStep = 'room';
      render();
    } catch (err) {
      if (errorEl) errorEl.textContent = err.message;
      else notify(err.message || 'Could not create the room.', 'error');
      if (btn) { btn.disabled = false; btn.textContent = 'Host This Quiz'; }
    } finally { adminInFlight = false; }
  }

  // ---- Room Lobby: Share code, see players, start game ----
  function renderRoomLobby(app) {
    const card = el('div', { className: 'card', style: 'text-align:center', 'data-testid': 'room-lobby' },
      el('h2', null, 'Room Ready!'),

      el('div', { className: 'room-code-display', 'data-testid': 'room-code-display' },
        el('p', { style: 'color:var(--text-secondary);font-size:0.85rem;margin-bottom:0.5rem' }, 'Share this code with players:'),
        el('div', { className: 'room-code-big', 'data-testid': 'room-code-big' }, roomCode),
        el('p', { style: 'color:var(--text-muted);font-size:0.8rem;margin-top:0.5rem;word-break:break-all' }, roomLink),
        el('button', { className: 'btn btn-secondary btn-sm', style: 'margin-top:0.75rem', onclick: () => { navigator.clipboard?.writeText(roomLink); } }, 'Copy Link'),
      ),

      el('div', { style: 'margin-top:2rem;text-align:left' },
        el('h3', null, 'Players Joined', el('span', { className: 'badge', 'data-testid': 'player-count-badge' }, String(players.length))),
        el('ul', { className: 'player-list', 'data-testid': 'player-list' },
          ...players.map(p => el('li', { className: 'player-chip' }, p.nickname)),
        ),
        players.length === 0 ? el('p', { className: 'empty-state' }, 'Waiting for players to join...') : null,
      ),

      el('div', { style: 'margin-top:2rem' },
        el('button', { className: 'btn btn-accent', style: 'width:100%;padding:1rem;font-size:1.1rem', 'data-testid': 'admin-start-btn', onclick: handleStart, disabled: players.length === 0 }, 'Start Game'),
      ),
    );
    app.appendChild(card);
  }

  function refreshPlayerSection() {
    const list = $('[data-testid="player-list"]');
    const badge = $('[data-testid="player-count-badge"]');
    if (list && adminStep === 'room') {
      list.innerHTML = '';
      players.forEach(p => list.appendChild(el('li', { className: 'player-chip' }, p.nickname)));
    }
    if (badge) badge.textContent = String(players.length);
    // Enable start button if players exist
    const startBtn = $('[data-testid="admin-start-btn"]');
    if (startBtn) startBtn.disabled = players.length === 0;
  }

  async function handleStart() {
    if (adminInFlight) return;
    adminInFlight = true;
    const btn = $('[data-testid="admin-start-btn"]');
    if (btn) { btn.disabled = true; btn.textContent = 'Starting…'; }
    try { await api('/api/game/start', { method: 'POST' }); }
    catch (e) { notify(e.message || 'Could not start the game.', 'error'); if (btn) { btn.disabled = false; btn.textContent = 'Start Game'; } }
    finally { adminInFlight = false; }
  }

  // ---- Countdown ----
  function renderCountdownView(app) {
    app.appendChild(el('div', { className: 'card', style: 'text-align:center' },
      el('h2', null, 'Game Starting!'),
      el('div', { className: 'countdown-circle' },
        el('span', { className: 'countdown-number', 'data-testid': 'admin-countdown-number' }, String(Math.max(0, countdownLeft)))),
      el('p', { style: 'color:var(--text-secondary)' }, `${totalQuestions} questions · ${players.length} players`),
    ));
  }

  // ---- Game View (question + reveal) ----
  function renderGameView(app) {
    const layout = el('div', { className: 'admin-layout' });

    // Left: Question
    const q = currentQuestion;
    const isReveal = adminStep === 'reveal';
    const optLabels = ['A','B','C','D','E','F'];

    const qCard = el('div', { className: 'card', 'data-testid': 'admin-question-card' },
      el('div', { className: 'question-meta' },
        el('span', { className: 'question-counter' }, `Question ${questionIndex + 1} of ${totalQuestions}`),
        el('span', { className: 'timer-number', 'data-testid': 'admin-timer-num' }, isReveal ? 'Time\'s up!' : (timeLeft + 's')),
      ),
      el('div', { className: 'timer-bar' },
        el('div', { className: 'timer-fill' + (isReveal ? ' critical' : ''), 'data-testid': 'admin-timer-fill',
          style: isReveal ? 'width:0%' : `width:${Math.max(0, (timeLeft / timeLimit) * 100)}%` })),
    );

    if (q) {
      qCard.appendChild(el('p', { className: 'question-text' }, q.text));
      qCard.appendChild(el('div', { className: 'options-grid' },
        ...q.options.map((opt, i) => {
          let cls = 'option-btn disabled';
          if (isReveal && i === correctAnswer) cls += ' correct';
          return el('div', { className: cls },
            el('span', { className: 'option-label' }, optLabels[i] || String(i)), opt);
        })
      ));
    }

    // Answer stats
    qCard.appendChild(el('div', { className: 'answer-stats', style: 'margin-top:1rem' },
      el('div', { className: 'stat-box' }, el('div', { className: 'stat-value', 'data-testid': 'stat-total' }, String(answerStats.total)), el('div', { className: 'stat-label' }, 'Answers')),
      el('div', { className: 'stat-box correct-stat' }, el('div', { className: 'stat-value', 'data-testid': 'stat-correct' }, String(answerStats.correct)), el('div', { className: 'stat-label' }, 'Correct')),
      el('div', { className: 'stat-box wrong-stat' }, el('div', { className: 'stat-value', 'data-testid': 'stat-wrong' }, String(answerStats.wrong)), el('div', { className: 'stat-label' }, 'Wrong')),
    ));

    if (isReveal) {
      qCard.appendChild(el('div', { style: 'margin-top:1rem;text-align:right' },
        el('button', { className: 'btn btn-accent', 'data-testid': 'admin-next-btn', onclick: handleNext }, 'Next Question')));
    }

    layout.appendChild(qCard);

    // Right: Leaderboard
    const lbCard = el('div', { className: 'card', 'data-testid': 'leaderboard-card' }, el('h3', null, 'Leaderboard'));
    const list = el('div', { 'data-testid': 'admin-leaderboard' });
    if (leaderboard.length === 0) list.appendChild(el('div', { className: 'empty-state' }, 'No scores yet'));
    else leaderboard.forEach((e, i) => {
      let rCls = 'lb-rank';
      if (i === 0) rCls += ' gold'; else if (i === 1) rCls += ' silver'; else if (i === 2) rCls += ' bronze';
      list.appendChild(el('div', { className: 'lb-entry' },
        el('span', { className: rCls }, `#${e.rank}`), el('span', { className: 'lb-name' }, e.nickname), el('span', { className: 'lb-score' }, String(e.score))));
    });
    lbCard.appendChild(list);
    layout.appendChild(lbCard);

    app.appendChild(layout);
  }

  function refreshStats() {
    const t = $('[data-testid="stat-total"]'); if (t) t.textContent = String(answerStats.total);
    const c = $('[data-testid="stat-correct"]'); if (c) c.textContent = String(answerStats.correct);
    const w = $('[data-testid="stat-wrong"]'); if (w) w.textContent = String(answerStats.wrong);
  }

  function refreshLeaderboard() {
    const card = $('[data-testid="leaderboard-card"]');
    if (!card) return;
    const list = $('[data-testid="admin-leaderboard"]');
    if (!list) return;
    list.innerHTML = '';
    if (leaderboard.length === 0) list.appendChild(el('div', { className: 'empty-state' }, 'No scores yet'));
    else leaderboard.forEach((e, i) => {
      let rCls = 'lb-rank';
      if (i === 0) rCls += ' gold'; else if (i === 1) rCls += ' silver'; else if (i === 2) rCls += ' bronze';
      list.appendChild(el('div', { className: 'lb-entry' },
        el('span', { className: rCls }, `#${e.rank}`), el('span', { className: 'lb-name' }, e.nickname), el('span', { className: 'lb-score' }, String(e.score))));
    });
  }

  async function handleNext() {
    const btn = $('[data-testid="admin-next-btn"]');
    if (btn) { btn.disabled = true; btn.textContent = 'Loading...'; }
    try { await api('/api/game/next', { method: 'POST' }); }
    catch (e) { notify(e.message || 'Could not advance the game.', 'error'); if (btn) { btn.disabled = false; btn.textContent = 'Next Question'; } }
  }

  // ---- Finished ----
  function renderFinishedView(app) {
    const card = el('div', { className: 'card', style: 'text-align:center' },
      el('h2', { style: 'margin-bottom:1.5rem' }, 'Game Over!'),
      el('div', { 'data-testid': 'final-leaderboard' },
        ...leaderboard.map((e, i) => {
          let rCls = 'lb-rank';
          if (i === 0) rCls += ' gold'; else if (i === 1) rCls += ' silver'; else if (i === 2) rCls += ' bronze';
          return el('div', { className: 'lb-entry' },
            el('span', { className: rCls }, `#${e.rank}`), el('span', { className: 'lb-name' }, e.nickname), el('span', { className: 'lb-score' }, String(e.score)));
        })),
      el('div', { style: 'margin-top:2rem' },
        el('button', { className: 'btn btn-accent', 'data-testid': 'new-quiz-btn', onclick: handleBackToLibrary }, '← Back to Library')),
    );
    app.appendChild(card);
  }

  async function resetSession() {
    await api('/api/game/reset', { method: 'POST' });
    backToLibrary();
  }

  async function handleBackToLibrary() {
    try { await resetSession(); } catch (err) { notify(err.message || 'Failed to reset session.', 'error'); }
  }

  async function handleResetSession() {
    const ok = window.confirm('Reset this quiz session and clear players, scores, and room data? Your saved quizzes are kept. This cannot be undone.');
    if (!ok) return;
    try { await resetSession(); } catch (err) { notify(err.message || 'Failed to reset session.', 'error'); }
  }

  document.addEventListener('DOMContentLoaded', render);
})();
