/* =========================================================
   QuizHub - app.js
   Client-side logic for the quiz game
   ========================================================= */

(function () {
  'use strict';

  // ---- State ----
  let playerId = null;
  let playerNickname = '';
  let gameState = null;
  let timerInterval = null;
  let selectedAnswer = null;
  let answerResult = null;
  let pollInterval = null;

  const API = '';  // same origin

  // ---- DOM helpers ----
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => document.querySelectorAll(sel);

  function el(tag, attrs, ...children) {
    const node = document.createElement(tag);
    if (attrs) {
      Object.entries(attrs).forEach(([k, v]) => {
        if (k === 'className') node.className = v;
        else if (k.startsWith('data-')) node.setAttribute(k, v);
        else if (k === 'onclick') node.addEventListener('click', v);
        else if (k === 'disabled') node.disabled = v;
        else node.setAttribute(k, v);
      });
    }
    children.flat().forEach(c => {
      if (c == null) return;
      node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return node;
  }

  // ---- API calls ----
  async function api(path, opts = {}) {
    const res = await fetch(API + path, {
      headers: { 'Content-Type': 'application/json' },
      ...opts,
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  // ---- Screens ----

  function renderApp() {
    const app = $('#app');
    app.innerHTML = '';

    // Header
    const header = el('header', { className: 'header' },
      el('h1', null, 'QuizHub'),
      el('p', null, playerId
        ? `Playing as ${playerNickname}`
        : 'Real-time multiplayer trivia')
    );
    app.appendChild(header);

    if (!playerId) {
      renderJoinScreen(app);
    } else if (!gameState || gameState.status === 'lobby') {
      renderLobby(app);
    } else if (gameState.status === 'question') {
      renderQuestion(app);
    } else if (gameState.status === 'finished') {
      renderLeaderboard(app);
    }
  }

  function renderJoinScreen(app) {
    let errorMsg = '';

    const card = el('div', { className: 'card join-screen', 'data-testid': 'join-screen' },
      el('h2', null, 'Join the Quiz'),
      el('p', { className: 'subtitle' }, 'Enter your nickname to get started'),
      el('div', { className: 'input-group' },
        el('input', {
          id: 'nickname-input',
          type: 'text',
          placeholder: 'Your nickname...',
          'data-testid': 'nickname-input',
          maxlength: '30',
        }),
        el('button', {
          className: 'btn btn-primary',
          'data-testid': 'join-btn',
          onclick: handleJoin,
        }, 'Join Game')
      ),
      el('p', { className: 'error-msg', 'data-testid': 'join-error' })
    );
    app.appendChild(card);

    // Focus input
    setTimeout(() => {
      const inp = $('#nickname-input');
      if (inp) {
        inp.focus();
        inp.addEventListener('keydown', (e) => {
          if (e.key === 'Enter') handleJoin();
        });
      }
    }, 50);
  }

  async function handleJoin() {
    const input = $('#nickname-input');
    const errorEl = $('[data-testid="join-error"]');
    const btn = $('[data-testid="join-btn"]');
    const nickname = input.value.trim();

    if (!nickname) {
      errorEl.textContent = 'Please enter a nickname';
      input.focus();
      return;
    }

    btn.disabled = true;
    btn.textContent = 'Joining...';
    errorEl.textContent = '';

    try {
      const data = await api('/api/join', {
        method: 'POST',
        body: JSON.stringify({ nickname }),
      });
      playerId = data.player_id;
      playerNickname = nickname;
      startPolling();
      renderApp();
    } catch (err) {
      errorEl.textContent = err.message || 'Failed to join';
      btn.disabled = false;
      btn.textContent = 'Join Game';
    }
  }

  async function renderLobby(app) {
    const card = el('div', { className: 'card lobby-screen', 'data-testid': 'lobby-screen' },
      el('h2', null, 'Lobby'),
      el('p', { className: 'lobby-info' }, 'Waiting for players to join...'),
      el('ul', { className: 'player-list', 'data-testid': 'player-list' }),
      el('div', { className: 'lobby-actions' },
        el('button', {
          className: 'btn btn-primary',
          'data-testid': 'start-game-btn',
          onclick: handleStartGame,
        }, 'Start Game'),
      )
    );
    app.appendChild(card);

    // Load players
    try {
      const players = await api('/api/players');
      const list = $('[data-testid="player-list"]');
      list.innerHTML = '';
      players.forEach(p => {
        const isYou = p.player_id === playerId;
        list.appendChild(
          el('li', {
            className: 'player-chip' + (isYou ? ' you' : ''),
            'data-testid': 'player-chip',
          }, p.nickname + (isYou ? ' (you)' : ''))
        );
      });
    } catch (_) {
      // silently fail, will retry on next poll
    }
  }

  async function handleStartGame() {
    const btn = $('[data-testid="start-game-btn"]');
    btn.disabled = true;
    btn.textContent = 'Starting...';

    try {
      gameState = await api('/api/game/start', { method: 'POST' });
      selectedAnswer = null;
      answerResult = null;
      renderApp();
    } catch (err) {
      btn.disabled = false;
      btn.textContent = 'Start Game';
      alert(err.message);
    }
  }

  function renderQuestion(app) {
    const q = gameState.current_question;
    if (!q) {
      app.appendChild(el('div', { className: 'card waiting' },
        el('div', { className: 'spinner' }),
        el('p', null, 'Loading question...')
      ));
      return;
    }

    const optionLabels = ['A', 'B', 'C', 'D', 'E', 'F'];

    const card = el('div', { className: 'card question-screen', 'data-testid': 'question-screen' },
      el('div', { className: 'question-meta' },
        el('span', { className: 'question-counter', 'data-testid': 'question-counter' },
          `Question ${gameState.question_index + 1} of ${gameState.total_questions}`),
        el('span', { className: 'category-tag', 'data-testid': 'category-tag' }, q.category || 'general'),
      ),
      el('div', { className: 'timer-bar' },
        el('div', {
          className: 'timer-fill',
          id: 'timer-fill',
          'data-testid': 'timer-fill',
        })
      ),
      el('p', { className: 'question-text', 'data-testid': 'question-text' }, q.text),
      el('div', { className: 'options-grid', 'data-testid': 'options-grid' },
        ...q.options.map((opt, i) => {
          let cls = 'option-btn';
          if (answerResult != null) {
            cls += ' disabled';
            if (i === answerResult.correct_answer) cls += ' correct';
            if (selectedAnswer === i && !answerResult.correct) cls += ' wrong';
          }
          return el('button', {
            className: cls,
            'data-testid': `option-${i}`,
            onclick: () => handleAnswer(i),
            disabled: answerResult != null,
          },
            el('span', { className: 'option-label' }, optionLabels[i] || String(i)),
            opt
          );
        })
      ),
    );

    // Show result toast if answered
    if (answerResult) {
      const isCorrect = answerResult.correct;
      card.appendChild(
        el('div', {
          className: 'result-toast ' + (isCorrect ? 'correct' : 'wrong'),
          'data-testid': 'result-toast',
        },
          el('span', null, isCorrect ? 'Correct!' : 'Wrong!'),
          el('span', { className: 'result-score' },
            isCorrect ? `+${answerResult.score_earned}` : '+0'),
        )
      );

      card.appendChild(
        el('div', { style: 'margin-top:1rem;text-align:right' },
          el('button', {
            className: 'btn btn-primary',
            'data-testid': 'next-question-btn',
            onclick: handleNextQuestion,
          }, 'Next')
        )
      );
    }

    app.appendChild(card);
    startTimer();
  }

  function startTimer() {
    clearInterval(timerInterval);
    if (answerResult) return; // don't run timer after answering

    const fill = $('#timer-fill');
    if (!fill || !gameState) return;

    const totalTime = gameState.time_left || 15;
    let timeLeft = totalTime;

    function update() {
      const pct = Math.max(0, (timeLeft / totalTime) * 100);
      fill.style.width = pct + '%';
      fill.classList.toggle('warning', timeLeft <= 5 && timeLeft > 2);
      fill.classList.toggle('critical', timeLeft <= 2);
    }

    update();

    timerInterval = setInterval(() => {
      timeLeft -= 1;
      update();
      if (timeLeft <= 0) {
        clearInterval(timerInterval);
        // Auto-submit wrong if not answered
        if (!answerResult) {
          handleAnswer(-1); // timeout: wrong answer
        }
      }
    }, 1000);
  }

  async function handleAnswer(index) {
    if (answerResult) return;
    clearInterval(timerInterval);

    selectedAnswer = index;

    try {
      answerResult = await api('/api/answer', {
        method: 'POST',
        body: JSON.stringify({
          player_id: playerId,
          question_id: gameState.current_question.id,
          answer: index,
        }),
      });
    } catch (err) {
      answerResult = { correct: false, correct_answer: -1, score_earned: 0, total_score: 0 };
    }

    renderApp();
  }

  async function handleNextQuestion() {
    try {
      gameState = await api('/api/game/next', { method: 'POST' });
      selectedAnswer = null;
      answerResult = null;
      renderApp();
    } catch (err) {
      if (err.message.includes('not active')) {
        gameState = { status: 'finished' };
        renderApp();
      }
    }
  }

  async function renderLeaderboard(app) {
    const card = el('div', { className: 'card leaderboard-screen', 'data-testid': 'leaderboard-screen' },
      el('h2', null, 'Game Over'),
      el('p', { className: 'leaderboard-subtitle' }, 'Final standings'),
      el('ul', { className: 'leaderboard-list', 'data-testid': 'leaderboard-list' }),
      el('div', { className: 'leaderboard-actions' },
        el('button', {
          className: 'btn btn-primary',
          'data-testid': 'play-again-btn',
          onclick: handlePlayAgain,
        }, 'Play Again'),
      )
    );
    app.appendChild(card);

    try {
      const entries = await api('/api/leaderboard');
      const list = $('[data-testid="leaderboard-list"]');
      list.innerHTML = '';
      entries.forEach((e, i) => {
        const isYou = e.player_id === playerId;
        let rankCls = 'rank';
        let entryCls = 'leaderboard-entry';
        if (i === 0) { rankCls += ' gold'; entryCls += ' top-1'; }
        else if (i === 1) { rankCls += ' silver'; entryCls += ' top-2'; }
        else if (i === 2) { rankCls += ' bronze'; entryCls += ' top-3'; }
        if (isYou) entryCls += ' you';

        list.appendChild(
          el('li', { className: entryCls, 'data-testid': `leaderboard-entry-${i}` },
            el('span', { className: rankCls }, `#${e.rank}`),
            el('span', { className: 'entry-name' }, e.nickname + (isYou ? ' (you)' : '')),
            el('span', { className: 'entry-score' }, String(e.score)),
          )
        );
      });
    } catch (_) {
      // retry on next render
    }
  }

  async function handlePlayAgain() {
    try {
      await api('/api/game/reset', { method: 'POST' });
      gameState = { status: 'lobby' };
      selectedAnswer = null;
      answerResult = null;
      // Keep player identity, re-join
      try {
        await api('/api/join', {
          method: 'POST',
          body: JSON.stringify({ nickname: playerNickname }),
        });
      } catch (_) {}
      renderApp();
    } catch (err) {
      alert('Failed to reset: ' + err.message);
    }
  }

  // ---- Polling ----
  function startPolling() {
    stopPolling();
    pollInterval = setInterval(async () => {
      try {
        const state = await api('/api/game/state');
        // Only update if state changed meaningfully
        if (!gameState || state.status !== gameState.status ||
            state.question_index !== gameState.question_index) {
          // Don't override during active answering
          if (answerResult && state.status === 'question' &&
              gameState && state.question_index === gameState.question_index) {
            return;
          }
          if (state.status !== gameState?.status || state.question_index !== gameState?.question_index) {
            selectedAnswer = null;
            answerResult = null;
          }
          gameState = state;
          renderApp();
        }
      } catch (_) {}
    }, 2000);
  }

  function stopPolling() {
    clearInterval(pollInterval);
  }

  // ---- Init ----
  document.addEventListener('DOMContentLoaded', () => {
    renderApp();
  });
})();
