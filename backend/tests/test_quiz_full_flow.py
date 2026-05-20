"""Regression tests for admin-to-player full room quiz flow."""

import os
import time

import pytest
import requests


BASE_URL = os.environ.get("REACT_APP_BACKEND_URL")


@pytest.fixture(scope="session")
def base_url() -> str:
    if not BASE_URL:
        pytest.skip("REACT_APP_BACKEND_URL is required for public endpoint testing")
    return BASE_URL.rstrip("/")


@pytest.fixture(scope="session")
def api_client() -> requests.Session:
    session = requests.Session()
    session.headers.update({"Content-Type": "application/json"})
    return session


def _assert_ok_json(resp: requests.Response, expected_status: int):
    assert resp.status_code == expected_status, f"{resp.status_code} != {expected_status}, body={resp.text}"
    return resp.json()


@pytest.fixture(scope="module")
def game_ctx(base_url: str, api_client: requests.Session):
    # Admin auth + clean reset for deterministic flow
    auth = api_client.post(f"{base_url}/api/admin/auth", json={"pin": "1234"})
    auth_data = _assert_ok_json(auth, 200)
    token = auth_data["token"]
    api_client.headers.update({"X-Admin-Token": token})

    reset = api_client.post(f"{base_url}/api/game/reset")
    _assert_ok_json(reset, 200)

    yield {"token": token}

    # Cleanup session state created by tests
    api_client.headers.update({"X-Admin-Token": token})
    api_client.post(f"{base_url}/api/game/reset")


def test_admin_adds_two_questions_and_room_creation_persists_selection(base_url: str, api_client: requests.Session, game_ctx):
    """Admin setup: add 2 questions and create room without question selection error."""
    q1 = {
        "text": "TEST_What color is the sky?",
        "options": ["Blue", "Green", "Red", "Yellow"],
        "answer": 0,
        "category": "custom",
    }
    q2 = {
        "text": "TEST_2 + 2 equals?",
        "options": ["3", "4", "5", "6"],
        "answer": 1,
        "category": "custom",
    }

    created1 = _assert_ok_json(api_client.post(f"{base_url}/api/questions/add", json=q1), 201)
    created2 = _assert_ok_json(api_client.post(f"{base_url}/api/questions/add", json=q2), 201)

    assert created1["text"] == q1["text"]
    assert created2["text"] == q2["text"]
    assert isinstance(created1["id"], int)
    assert isinstance(created2["id"], int)

    room = _assert_ok_json(api_client.post(f"{base_url}/api/room/create"), 201)
    assert room["room_code"]
    assert room["link"]


def test_two_players_join_lobby(base_url: str, api_client: requests.Session):
    """Player join flow: two players should join with generated room code."""
    state = _assert_ok_json(api_client.get(f"{base_url}/api/game/state"), 200)
    room_code = state.get("room_code")
    assert room_code, "room_code missing after room creation"

    p1 = _assert_ok_json(
        api_client.post(
            f"{base_url}/api/join",
            json={"nickname": "TEST_Player_One", "room_code": room_code},
        ),
        201,
    )
    p2 = _assert_ok_json(
        api_client.post(
            f"{base_url}/api/join",
            json={"nickname": "TEST_Player_Two", "room_code": room_code},
        ),
        201,
    )

    assert p1["nickname"] == "TEST_Player_One"
    assert p2["nickname"] == "TEST_Player_Two"
    assert p1["player_id"] != p2["player_id"]

    players = _assert_ok_json(api_client.get(f"{base_url}/api/players"), 200)
    names = {p["nickname"] for p in players}
    assert "TEST_Player_One" in names
    assert "TEST_Player_Two" in names


def test_start_game_with_added_questions_and_complete_two_question_round(base_url: str, api_client: requests.Session):
    """Game flow: start -> question1 answer -> question2 answer -> finished leaderboard."""
    players = _assert_ok_json(api_client.get(f"{base_url}/api/players"), 200)
    player_ids = [p["player_id"] for p in players if p["nickname"].startswith("TEST_Player_")]
    assert len(player_ids) == 2, f"expected 2 test players, got {len(player_ids)}"

    start = _assert_ok_json(api_client.post(f"{base_url}/api/game/start"), 200)
    assert start["status"] == "countdown"
    assert start["total_questions"] == 2

    # Wait until countdown transitions to active question
    time.sleep(11)
    q_state = _assert_ok_json(api_client.get(f"{base_url}/api/game/state"), 200)
    assert q_state["status"] == "question", q_state
    q1_id = q_state["current_question"]["id"]

    for player_id in player_ids:
        ans = _assert_ok_json(
            api_client.post(
                f"{base_url}/api/answer",
                json={"player_id": player_id, "question_id": q1_id, "answer": 0},
            ),
            200,
        )
        assert ans["recorded"] is True

    # Move to reveal quickly and then next question
    time.sleep(16)
    reveal = _assert_ok_json(api_client.get(f"{base_url}/api/game/state"), 200)
    assert reveal["status"] == "reveal"

    next_resp = _assert_ok_json(api_client.post(f"{base_url}/api/game/next"), 200)
    assert next_resp["status"] == "question"

    q2_state = _assert_ok_json(api_client.get(f"{base_url}/api/game/state"), 200)
    assert q2_state["status"] == "question"
    q2_id = q2_state["current_question"]["id"]
    assert q2_id != q1_id

    for player_id in player_ids:
        ans2 = _assert_ok_json(
            api_client.post(
                f"{base_url}/api/answer",
                json={"player_id": player_id, "question_id": q2_id, "answer": 1},
            ),
            200,
        )
        assert ans2["recorded"] is True

    time.sleep(16)
    reveal2 = _assert_ok_json(api_client.get(f"{base_url}/api/game/state"), 200)
    assert reveal2["status"] == "reveal"

    finished = _assert_ok_json(api_client.post(f"{base_url}/api/game/next"), 200)
    assert finished["status"] == "finished"

    leaderboard = _assert_ok_json(api_client.get(f"{base_url}/api/leaderboard"), 200)
    lb_names = [entry["nickname"] for entry in leaderboard]
    assert "TEST_Player_One" in lb_names
    assert "TEST_Player_Two" in lb_names
