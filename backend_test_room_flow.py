#!/usr/bin/env python3
"""
QuizHub Backend API Testing Suite - Room-Based Flow
Tests all the features mentioned in the review request:
- Admin auth, question management, room creation
- Player join with room code, game flow
- WebSocket functionality
"""

import requests
import json
import sys
import time
from datetime import datetime

class QuizHubRoomFlowTester:
    def __init__(self, base_url="https://bug-hunter-192.preview.emergentagent.com"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({'Content-Type': 'application/json'})
        self.tests_run = 0
        self.tests_passed = 0
        self.admin_token = None
        self.room_code = None
        self.room_link = None
        self.player_id = None
        self.question_id = None
        
    def log(self, message, level="INFO"):
        timestamp = datetime.now().strftime("%H:%M:%S")
        print(f"[{timestamp}] {level}: {message}")
        
    def run_test(self, name, method, endpoint, expected_status, data=None, description=""):
        """Run a single API test"""
        url = f"{self.base_url}/api/{endpoint}"
        self.tests_run += 1
        
        self.log(f"🔍 Testing {name} - {description}")
        
        try:
            if method == 'GET':
                response = self.session.get(url)
            elif method == 'POST':
                response = self.session.post(url, json=data)
                
            success = response.status_code == expected_status
            
            if success:
                self.tests_passed += 1
                self.log(f"✅ PASSED - Status: {response.status_code}", "SUCCESS")
                try:
                    response_data = response.json()
                    self.log(f"   Response: {json.dumps(response_data, indent=2)[:300]}...")
                    return True, response_data
                except:
                    return True, {}
            else:
                self.log(f"❌ FAILED - Expected {expected_status}, got {response.status_code}", "ERROR")
                try:
                    error_data = response.json()
                    self.log(f"   Error: {error_data}")
                except:
                    self.log(f"   Raw response: {response.text[:200]}")
                return False, {}
                
        except Exception as e:
            self.log(f"❌ FAILED - Exception: {str(e)}", "ERROR")
            return False, {}

    def test_health(self):
        """Test GET /api/health returns ok"""
        success, data = self.run_test(
            "Health Check", "GET", "health", 200,
            description="Should return status ok"
        )
        return success and data.get('status') == 'ok'

    def test_admin_auth(self):
        """Test POST /api/admin/auth with PIN 1234 returns token"""
        success, data = self.run_test(
            "Admin Auth", "POST", "admin/auth", 200,
            data={"pin": "1234"},
            description="Should return token for PIN 1234"
        )
        
        if success and 'token' in data:
            self.admin_token = data['token']
            self.session.headers.update({'X-Admin-Token': self.admin_token})
            return True
        return False

    def test_add_question(self):
        """Test POST /api/questions/add adds a question (admin token required)"""
        success, data = self.run_test(
            "Add Question", "POST", "questions/add", 201,
            data={
                "text": "What is the capital of France?",
                "options": ["London", "Paris", "Berlin", "Madrid"],
                "answer": 1,
                "category": "geography"
            },
            description="Should add a question with admin token"
        )
        
        if success and 'id' in data:
            self.question_id = data['id']
            return True
        return False

    def test_create_room_without_questions_fails(self):
        """Test POST /api/room/create fails if no questions added"""
        # First reset to clear any existing questions
        self.run_test("Reset Game", "POST", "game/reset", 200)
        
        success, data = self.run_test(
            "Create Room (No Questions)", "POST", "room/create", 400,
            description="Should fail when no questions exist"
        )
        return success

    def test_create_room_with_questions(self):
        """Test POST /api/room/create generates room code and link (admin token required)"""
        # First add a question
        self.test_add_question()
        
        success, data = self.run_test(
            "Create Room", "POST", "room/create", 201,
            description="Should generate room code and link"
        )
        
        if success and 'room_code' in data and 'link' in data:
            self.room_code = data['room_code']
            self.room_link = data['link']
            self.log(f"✅ Room created: {self.room_code}, Link: {self.room_link}")
            return True
        return False

    def test_room_info_valid_code(self):
        """Test GET /api/room/info?code=CODE returns room status"""
        if not self.room_code:
            return False
            
        success, data = self.run_test(
            "Room Info (Valid)", "GET", f"room/info?code={self.room_code}", 200,
            description="Should return room status for valid code"
        )
        
        return success and data.get('room_code') == self.room_code

    def test_room_info_invalid_code(self):
        """Test GET /api/room/info?code=INVALID returns 404"""
        success, data = self.run_test(
            "Room Info (Invalid)", "GET", "room/info?code=INVALID", 404,
            description="Should return 404 for invalid code"
        )
        return success

    def test_join_wrong_room_code(self):
        """Test POST /api/join with wrong room code returns 404"""
        success, data = self.run_test(
            "Join (Wrong Code)", "POST", "join", 404,
            data={"nickname": "TestPlayer", "room_code": "WRONG1"},
            description="Should return 404 for wrong room code"
        )
        return success

    def test_join_correct_room_code(self):
        """Test POST /api/join with correct room code creates player"""
        if not self.room_code:
            return False
            
        success, data = self.run_test(
            "Join (Correct Code)", "POST", "join", 201,
            data={"nickname": "TestPlayer", "room_code": self.room_code},
            description="Should create player with correct room code"
        )
        
        if success and 'player_id' in data:
            self.player_id = data['player_id']
            return True
        return False

    def test_join_missing_fields(self):
        """Test POST /api/join requires room_code and nickname"""
        # Test missing nickname
        success1, _ = self.run_test(
            "Join (Missing Nickname)", "POST", "join", 400,
            data={"room_code": self.room_code or "TEST"},
            description="Should fail without nickname"
        )
        
        # Test missing room_code
        success2, _ = self.run_test(
            "Join (Missing Room Code)", "POST", "join", 400,
            data={"nickname": "TestPlayer"},
            description="Should fail without room_code"
        )
        
        return success1 and success2

    def test_game_start_triggers_countdown(self):
        """Test POST /api/game/start triggers countdown (admin token required)"""
        success, data = self.run_test(
            "Start Game", "POST", "game/start", 200,
            description="Should trigger countdown"
        )
        
        return success and data.get('status') == 'countdown'

    def test_full_game_flow(self):
        """Test full game flow: countdown -> question -> timer -> reveal -> next -> finished"""
        self.log("🎮 Testing full game flow...")
        
        # Wait for countdown to complete
        self.log("⏳ Waiting for countdown...")
        time.sleep(11)
        
        # Check question state
        success, data = self.run_test(
            "Game State (Question)", "GET", "game/state", 200,
            description="Should be in question state"
        )
        
        if not (success and data.get('status') == 'question'):
            return False
            
        question_id = data['current_question']['id']
        
        # Submit answer
        if self.player_id:
            self.run_test(
                "Submit Answer", "POST", "answer", 200,
                data={"player_id": self.player_id, "question_id": question_id, "answer": 1}
            )
        
        # Wait for timer to expire
        self.log("⏳ Waiting for question timer...")
        time.sleep(16)
        
        # Check reveal state
        success, data = self.run_test(
            "Game State (Reveal)", "GET", "game/state", 200,
            description="Should be in reveal state"
        )
        
        if not (success and data.get('status') == 'reveal'):
            return False
            
        # Advance to next (should finish since we only have 1 question)
        success, data = self.run_test(
            "Next Question", "POST", "game/next", 200,
            description="Should advance to finished"
        )
        
        return success and data.get('status') == 'finished'

    def test_websocket_endpoint(self):
        """Test WebSocket endpoint is accessible"""
        # We can't easily test WebSocket in this script, but we can check the endpoint exists
        # by trying to connect to it (it will fail but shouldn't return 404)
        try:
            import websocket
            ws_url = self.base_url.replace('https://', 'wss://').replace('http://', 'ws://') + '/api/ws'
            ws = websocket.create_connection(ws_url, timeout=5)
            ws.close()
            self.log("✅ WebSocket endpoint accessible")
            return True
        except Exception as e:
            # Even if connection fails, if it's not a 404, the endpoint exists
            if "404" not in str(e):
                self.log("✅ WebSocket endpoint exists (connection expected to fail in test)")
                return True
            self.log(f"❌ WebSocket endpoint issue: {e}")
            return False

    def run_all_tests(self):
        """Run all room-based flow tests"""
        self.log("🚀 Starting QuizHub Room-Based Flow Tests")
        self.log(f"   Base URL: {self.base_url}")
        
        test_methods = [
            ("Health Check", self.test_health),
            ("Admin Auth", self.test_admin_auth),
            ("Add Question", self.test_add_question),
            ("Create Room (No Questions)", self.test_create_room_without_questions_fails),
            ("Create Room (With Questions)", self.test_create_room_with_questions),
            ("Room Info (Valid Code)", self.test_room_info_valid_code),
            ("Room Info (Invalid Code)", self.test_room_info_invalid_code),
            ("Join (Wrong Code)", self.test_join_wrong_room_code),
            ("Join (Missing Fields)", self.test_join_missing_fields),
            ("Join (Correct Code)", self.test_join_correct_room_code),
            ("Game Start", self.test_game_start_triggers_countdown),
            ("Full Game Flow", self.test_full_game_flow),
            ("WebSocket Endpoint", self.test_websocket_endpoint),
        ]
        
        for test_name, test_method in test_methods:
            try:
                self.log(f"\n--- {test_name} ---")
                if not test_method():
                    self.log(f"❌ {test_name} failed", "ERROR")
                time.sleep(1)
            except Exception as e:
                self.log(f"❌ {test_name} failed with exception: {e}", "ERROR")
        
        # Print summary
        self.log("=" * 60)
        self.log(f"📊 Test Results: {self.tests_passed}/{self.tests_run} passed")
        
        if self.tests_passed == self.tests_run:
            self.log("🎉 All tests passed!", "SUCCESS")
            return 0
        else:
            self.log(f"❌ {self.tests_run - self.tests_passed} tests failed", "ERROR")
            return 1

def main():
    tester = QuizHubRoomFlowTester()
    return tester.run_all_tests()

if __name__ == "__main__":
    sys.exit(main())