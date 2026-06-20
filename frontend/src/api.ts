import type { Entry, Food, Goals, Meal } from './types'

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: 'Unbekannter Fehler' }))
    throw new Error(body.error || 'Anfrage fehlgeschlagen')
  }
  if (response.status === 204) return undefined as T
  return response.json()
}

export const api = {
  foods: (query = '') =>
    request<Food[]>(`/api/foods?q=${encodeURIComponent(query)}`),
  barcode: (code: string) =>
    request<Food>(`/api/foods/barcode/${encodeURIComponent(code)}`),
  createFood: (food: Omit<Food, 'id' | 'source' | 'imageUrl'>) =>
    request<Food>('/api/foods', {
      method: 'POST',
      body: JSON.stringify({ ...food, source: 'manual', imageUrl: '' }),
    }),
  updateServing: (foodId: number, servingSize: number) =>
    request<Food>(`/api/foods/${foodId}/serving`, {
      method: 'PUT',
      body: JSON.stringify({ servingSize }),
    }),
  entries: (date: string) =>
    request<Entry[]>(`/api/entries?date=${encodeURIComponent(date)}`),
  createEntry: (foodId: number, date: string, meal: Meal, amount: number, quantity: number, unitAmount: number) =>
    request<{ id: number }>('/api/entries', {
      method: 'POST',
      body: JSON.stringify({ foodId, date, meal, amount, quantity, unitAmount }),
    }),
  updateEntry: (id: number, meal: Meal, amount: number, quantity: number, unitAmount: number) =>
    request<void>(`/api/entries/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ meal, amount, quantity, unitAmount }),
    }),
  deleteEntry: (id: number) =>
    request<void>(`/api/entries/${id}`, { method: 'DELETE' }),
  goals: () => request<Goals>('/api/goals'),
  updateGoals: (goals: Goals) =>
    request<Goals>('/api/goals', {
      method: 'PUT',
      body: JSON.stringify(goals),
    }),
}
