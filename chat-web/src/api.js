const API_BASE = '';

export const request = async (url, options = {}) => {
  const token = localStorage.getItem('token');
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  try {
    const res = await fetch(`${API_BASE}${url}`, {
      ...options,
      headers,
    });

    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user_info');
      window.location.href = '/login';
      return null;
    }

    const data = await res.json();
    return data;
  } catch (error) {
    console.error(`API Error on ${url}:`, error);
    return null;
  }
};
