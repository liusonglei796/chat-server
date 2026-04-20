#!/bin/sh
echo "--- REGISTER ---"
wget -qO- --post-data='{"username":"newuser001","password":"test123456"}' --header='Content-Type: application/json' http://localhost:8000/auth/register
echo ""
echo "--- LOGIN ---"
wget -qO- --post-data='{"username":"newuser001","password":"test123456"}' --header='Content-Type: application/json' http://localhost:8000/auth/login
