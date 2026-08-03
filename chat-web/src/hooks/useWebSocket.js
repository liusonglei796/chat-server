import { useState, useEffect, useRef, useCallback } from 'react';

export function useWebSocket(url) {
  const [messages, setMessages] = useState([]);
  const [status, setStatus] = useState('connecting');
  const ws = useRef(null);

  const connect = useCallback(() => {
    const token = localStorage.getItem('token');
    if (!token) return;

    setStatus('connecting');
    const wsUrl = `ws://${window.location.host}${url}?token=${token}`;
    ws.current = new WebSocket(wsUrl);

    ws.current.onopen = () => {
      console.log('WS Connected');
      setStatus('connected');
    };

    ws.current.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        setMessages(prev => [...prev, data]);
      } catch (err) {
        console.error('WS Parse Error', err);
      }
    };

    ws.current.onclose = () => {
      console.log('WS Disconnected');
      setStatus('disconnected');
      // Auto reconnect
      setTimeout(connect, 3000);
    };

    ws.current.onerror = (err) => {
      console.error('WS Error', err);
      ws.current.close();
    };
  }, [url]);

  useEffect(() => {
    connect();
    return () => {
      if (ws.current) {
        ws.current.close();
      }
    };
  }, [connect]);

  const sendMessage = useCallback((msgObj) => {
    if (ws.current && ws.current.readyState === WebSocket.OPEN) {
      ws.current.send(JSON.stringify(msgObj));
    } else {
      console.error('WS is not connected');
    }
  }, []);

  return { messages, sendMessage, status, setMessages };
}
