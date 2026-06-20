import { useEffect, useMemo, useState } from 'react'
import {
  Barcode, BookOpen, ChevronLeft, ChevronRight, CirclePlus, Flame, Leaf,
  LoaderCircle, Minus, PencilLine, Plus, ScanLine, Settings2, Trash2, X,
} from 'lucide-react'
import { api } from './api'
import type { Entry, Food, Goals, Meal, Totals } from './types'

const meals: { id: Meal; label: string; icon: string }[] = [
  { id: 'breakfast', label: 'Frühstück', icon: '☀️' },
  { id: 'lunch', label: 'Mittagessen', icon: '🥗' },
  { id: 'dinner', label: 'Abendessen', icon: '🌙' },
  { id: 'snack', label: 'Snacks', icon: '🍎' },
  { id: 'drinks', label: 'Getränke', icon: '🥤' },
]

const emptyGoals: Goals = { calories: 2200, protein: 140, carbs: 250, fat: 70, fiber: 30 }

function localDate(date = new Date()) {
  const offset = date.getTimezoneOffset()
  return new Date(date.getTime() - offset * 60_000).toISOString().slice(0, 10)
}

function shiftDate(value: string, days: number) {
  const date = new Date(`${value}T12:00:00`)
  date.setDate(date.getDate() + days)
  return localDate(date)
}

function formatDate(value: string) {
  const today = localDate()
  if (value === today) return 'Heute'
  if (value === shiftDate(today, -1)) return 'Gestern'
  return new Intl.DateTimeFormat('de-DE', {
    weekday: 'short', day: '2-digit', month: 'short',
  }).format(new Date(`${value}T12:00:00`))
}

function round(value: number, digits = 0) {
  const factor = 10 ** digits
  return Math.round(value * factor) / factor
}

function entryAmountLabel(entry: Entry) {
  const wholeQuantity = Math.round(entry.quantity)
  if (wholeQuantity > 1 && Math.abs(entry.quantity - wholeQuantity) < 0.001) {
    return `${wholeQuantity} × ${round(entry.unitAmount, 1)} g = ${round(entry.amount, 1)} g`
  }
  return `${round(entry.amount, 1)} g`
}

function totalsFor(entries: Entry[]): Totals {
  const totals: Totals = {
    calories: 0, protein: 0, carbs: 0, fat: 0, fiber: 0,
    sugar: 0, saturatedFat: 0, salt: 0, micros: {},
  }
  for (const entry of entries) {
    const factor = entry.amount / 100
    const food = entry.food
    totals.calories += food.calories * factor
    totals.protein += food.protein * factor
    totals.carbs += food.carbs * factor
    totals.fat += food.fat * factor
    totals.fiber += food.fiber * factor
    totals.sugar += food.sugar * factor
    totals.saturatedFat += food.saturatedFat * factor
    totals.salt += food.salt * factor
    for (const [name, value] of Object.entries(food.micros || {})) {
      totals.micros[name] = (totals.micros[name] || 0) + value * factor
    }
  }
  return totals
}

export default function App() {
  const [date, setDate] = useState(localDate())
  const [entries, setEntries] = useState<Entry[]>([])
  const [goals, setGoals] = useState<Goals>(emptyGoals)
  const [loading, setLoading] = useState(true)
  const [addMeal, setAddMeal] = useState<Meal | null>(null)
  const [editingEntry, setEditingEntry] = useState<Entry | null>(null)
  const [foodsOpen, setFoodsOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    try {
      const [newEntries, newGoals] = await Promise.all([api.entries(date), api.goals()])
      setEntries(newEntries)
      setGoals(newGoals)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Laden fehlgeschlagen')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [date])
  const totals = useMemo(() => totalsFor(entries), [entries])

  const removeEntry = async (id: number) => {
    try {
      await api.deleteEntry(id)
      setEntries(current => current.filter(entry => entry.id !== id))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Löschen fehlgeschlagen')
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="/">
          <span className="brand-mark"><Leaf size={22} strokeWidth={2.4} /></span>
          <span>Mampf<span>Tracker</span></span>
        </a>
        <div className="topbar-actions">
          <button className="icon-button" onClick={() => setFoodsOpen(true)} aria-label="Lebensmittel verwalten" title="Lebensmittel">
            <BookOpen size={21} />
          </button>
          <button className="icon-button" onClick={() => setSettingsOpen(true)} aria-label="Ziele öffnen" title="Tagesziele">
            <Settings2 size={21} />
          </button>
        </div>
      </header>

      <main>
        <div className="date-nav">
          <button className="icon-button" onClick={() => setDate(shiftDate(date, -1))}><ChevronLeft /></button>
          <button className="date-title" onClick={() => setDate(localDate())}>
            <strong>{formatDate(date)}</strong>
            <span>{new Intl.DateTimeFormat('de-DE', { day: '2-digit', month: 'long', year: 'numeric' }).format(new Date(`${date}T12:00:00`))}</span>
          </button>
          <button className="icon-button" onClick={() => setDate(shiftDate(date, 1))}><ChevronRight /></button>
        </div>

        {error && <div className="error-banner">{error}<button onClick={() => setError('')}><X size={17} /></button></div>}

        <section className="summary-grid">
          <CalorieCard current={totals.calories} goal={goals.calories} />
          <MacroCard label="Protein" current={totals.protein} goal={goals.protein} color="var(--protein)" />
          <MacroCard label="Kohlenhydrate" current={totals.carbs} goal={goals.carbs} color="var(--carbs)" />
          <MacroCard label="Fett" current={totals.fat} goal={goals.fat} color="var(--fat)" />
          <MacroCard label="Ballaststoffe" current={totals.fiber} goal={goals.fiber} color="var(--fiber)" />
        </section>

        {loading ? (
          <div className="loading-state"><LoaderCircle className="spin" /> Dein Tag wird angerichtet …</div>
        ) : (
          <section className="meals">
            <div className="section-heading">
              <div><span>Tagesprotokoll</span><h1>Was gab’s zu mampfen?</h1></div>
              <span className="entry-count">{entries.length} {entries.length === 1 ? 'Eintrag' : 'Einträge'}</span>
            </div>
            {meals.map(meal => (
              <MealSection
                key={meal.id}
                meal={meal}
                entries={entries.filter(entry => entry.meal === meal.id)}
                onAdd={() => setAddMeal(meal.id)}
                onEdit={setEditingEntry}
                onDelete={removeEntry}
              />
            ))}
          </section>
        )}

        <NutrientDetails totals={totals} />
      </main>

      {addMeal && (
        <AddDialog
          date={date}
          meal={addMeal}
          onClose={() => setAddMeal(null)}
          onAdded={async () => { setAddMeal(null); await load() }}
        />
      )}
      {editingEntry && (
        <EditEntryDialog
          entry={editingEntry}
          onClose={() => setEditingEntry(null)}
          onSaved={(meal, amount, quantity, unitAmount) => {
            setEntries(current => current.map(entry =>
              entry.id === editingEntry.id ? { ...entry, meal, amount, quantity, unitAmount } : entry))
            setEditingEntry(null)
          }}
        />
      )}
      {foodsOpen && <FoodLibrary onClose={() => setFoodsOpen(false)} onFoodUpdated={() => void load()} />}
      {settingsOpen && (
        <GoalsDialog goals={goals} onClose={() => setSettingsOpen(false)}
          onSave={newGoals => { setGoals(newGoals); setSettingsOpen(false) }} />
      )}
    </div>
  )
}

const editableMicros = [
  'Natrium', 'Calcium', 'Eisen', 'Magnesium', 'Kalium', 'Zink',
  'Vitamin C', 'Vitamin B12', 'Vitamin D',
]

function FoodLibrary({ onClose, onFoodUpdated }: { onClose: () => void; onFoodUpdated: () => void }) {
  const [foods, setFoods] = useState<Food[]>([])
  const [query, setQuery] = useState('')
  const [editing, setEditing] = useState<Food | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const loadFoods = async (search = '') => {
    setLoading(true)
    try {
      setFoods(await api.foods(search))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Lebensmittel konnten nicht geladen werden')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    const timer = window.setTimeout(() => void loadFoods(query), 180)
    return () => window.clearTimeout(timer)
  }, [query])

  return (
    <div className="dialog-backdrop" onMouseDown={event => event.target === event.currentTarget && onClose()}>
      <div className="dialog food-library-dialog">
        <div className="dialog-head">
          <div><span>Lebensmittelverwaltung</span><h2>{editing ? 'Lebensmittel bearbeiten' : 'Bekannte Lebensmittel'}</h2></div>
          <button className="icon-button" onClick={onClose}><X /></button>
        </div>
        {editing ? (
          <EditFoodForm
            food={editing}
            onCancel={() => setEditing(null)}
            onSaved={updated => {
              setFoods(current => current.map(food => food.id === updated.id ? updated : food))
              setEditing(null)
              onFoodUpdated()
            }}
          />
        ) : (
          <>
            <div className="search-box">
              <BookOpen />
              <input autoFocus placeholder="Name, Marke oder Barcode suchen …"
                value={query} onChange={event => setQuery(event.target.value)} />
            </div>
            <p className="library-note">
              Änderungen an Namen und Nährwerten gelten auch rückwirkend für alle Mahlzeiteneinträge.
            </p>
            {loading ? (
              <div className="loading-state small"><LoaderCircle className="spin" /> Lebensmittel werden geladen …</div>
            ) : (
              <div className="food-library-list">
                {foods.map(food => (
                  <button key={food.id} onClick={() => setEditing(food)}>
                    <span className="food-thumb">{food.imageUrl ? <img src={food.imageUrl} alt="" /> : food.name[0]}</span>
                    <span className="library-food-copy">
                      <strong>{food.name}</strong>
                      <small>{food.brand || 'Ohne Marke'} · {round(food.calories)} kcal / 100 g</small>
                      <small>Standardmenge {round(food.servingSize, 1)} g{food.barcode ? ` · ${food.barcode}` : ''}</small>
                    </span>
                    <PencilLine size={17} />
                  </button>
                ))}
                {foods.length === 0 && <p className="muted center">Keine Lebensmittel gefunden.</p>}
              </div>
            )}
          </>
        )}
        {error && <div className="inline-error">{error}</div>}
      </div>
    </div>
  )
}

function EditFoodForm({ food, onCancel, onSaved }: {
  food: Food
  onCancel: () => void
  onSaved: (food: Food) => void
}) {
  const [form, setForm] = useState<Food>(() => ({
    ...food,
    micros: Object.fromEntries(editableMicros.map(name => [name, food.micros?.[name] || 0])),
  }))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const setText = (key: 'name' | 'brand' | 'barcode', value: string) =>
    setForm(current => ({ ...current, [key]: value }))
  const setNumber = (key: keyof Food, value: string) =>
    setForm(current => ({ ...current, [key]: Number(value) }))

  const save = async (event: React.FormEvent) => {
    event.preventDefault()
    setBusy(true); setError('')
    try {
      const updated = await api.updateFood({
        ...form,
        micros: Object.fromEntries(Object.entries(form.micros).filter(([, value]) => value > 0)),
      })
      onSaved(updated)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Lebensmittel konnte nicht gespeichert werden')
      setBusy(false)
    }
  }

  return (
    <form className="edit-food-form" onSubmit={save}>
      <button type="button" className="back-link" onClick={onCancel}><ChevronLeft size={17} /> Zurück zur Übersicht</button>
      <div className="field-grid">
        <label className="field wide"><span>Name *</span>
          <input required value={form.name} onChange={event => setText('name', event.target.value)} />
        </label>
        <label className="field"><span>Marke</span>
          <input value={form.brand} onChange={event => setText('brand', event.target.value)} />
        </label>
        <label className="field"><span>Barcode</span>
          <input inputMode="numeric" value={form.barcode} onChange={event => setText('barcode', event.target.value)} />
        </label>
        <label className="field wide"><span>Standardmenge (g)</span>
          <input type="number" min="0.1" max="10000" step="0.1" value={form.servingSize}
            onChange={event => setNumber('servingSize', event.target.value)} />
        </label>
        <div className="form-divider wide">Nährwerte pro 100 g</div>
        {([
          ['calories', 'Kalorien', 'kcal'], ['protein', 'Protein', 'g'], ['carbs', 'Kohlenhydrate', 'g'],
          ['fat', 'Fett', 'g'], ['fiber', 'Ballaststoffe', 'g'], ['sugar', 'Zucker', 'g'],
          ['saturatedFat', 'Gesättigte Fettsäuren', 'g'], ['salt', 'Salz', 'g'],
        ] as const).map(([key, label, unit]) => (
          <label className="field" key={key}><span>{label} ({unit})</span>
            <input type="number" min="0" step="0.01" value={form[key]}
              onChange={event => setNumber(key, event.target.value)} />
          </label>
        ))}
        <div className="form-divider wide">Mikronährstoffe pro 100 g (mg)</div>
        {editableMicros.map(name => (
          <label className="field" key={name}><span>{name} (mg)</span>
            <input type="number" min="0" step="0.001" value={form.micros[name] || 0}
              onChange={event => setForm(current => ({
                ...current,
                micros: { ...current.micros, [name]: Number(event.target.value) },
              }))} />
          </label>
        ))}
      </div>
      <p className="library-warning">
        Historische Gramm- und Anzahlwerte bleiben unverändert. Ihre Kalorien und Nährwerte werden mit diesen Stammdaten neu berechnet.
      </p>
      {error && <div className="inline-error">{error}</div>}
      <button className="primary-button full" disabled={busy}>
        {busy ? <LoaderCircle className="spin" /> : <PencilLine />} Lebensmittel speichern
      </button>
    </form>
  )
}

function CalorieCard({ current, goal }: { current: number; goal: number }) {
  const ratio = Math.min(current / goal, 1)
  const remaining = Math.max(goal - current, 0)
  return (
    <article className="calorie-card">
      <div className="ring" style={{ '--progress': `${ratio * 360}deg` } as React.CSSProperties}>
        <div><Flame size={20} fill="currentColor" /><strong>{round(current)}</strong><span>kcal</span></div>
      </div>
      <div className="calorie-copy">
        <span>Kalorien</span>
        <h2>{round(remaining)} übrig</h2>
        <p>von {round(goal)} kcal Tagesziel</p>
      </div>
    </article>
  )
}

function MacroCard({ label, current, goal, color }: {
  label: string; current: number; goal: number; color: string
}) {
  const ratio = Math.min(current / (goal || 1), 1)
  return (
    <article className="macro-card">
      <div className="macro-top"><span>{label}</span><strong>{round(current, 1)}<small> / {goal} g</small></strong></div>
      <div className="progress"><span style={{ width: `${ratio * 100}%`, background: color }} /></div>
      <small>{round(ratio * 100)} % erreicht</small>
    </article>
  )
}

function MealSection({ meal, entries, onAdd, onEdit, onDelete }: {
  meal: typeof meals[number]
  entries: Entry[]
  onAdd: () => void
  onEdit: (entry: Entry) => void
  onDelete: (id: number) => void
}) {
  const calories = totalsFor(entries).calories
  return (
    <article className="meal-card">
      <div className="meal-header">
        <div><span className="meal-icon">{meal.icon}</span><h2>{meal.label}</h2><small>{round(calories)} kcal</small></div>
        <button className="add-button" onClick={onAdd}><Plus size={18} /> Hinzufügen</button>
      </div>
      {entries.length === 0 ? (
        <button className="empty-meal" onClick={onAdd}><CirclePlus size={19} /> Noch nichts eingetragen</button>
      ) : (
        <div className="entry-list">
          {entries.map(entry => (
            <div className="entry-row" key={entry.id}>
              <div className="food-thumb">
                {entry.food.imageUrl ? <img src={entry.food.imageUrl} alt="" /> : entry.food.name.slice(0, 1).toUpperCase()}
              </div>
              <div className="entry-name">
                <strong>{entry.food.name}</strong>
                <span>{entry.food.brand || 'Eigenes Lebensmittel'} · {entryAmountLabel(entry)}</span>
              </div>
              <div className="entry-nutrition">
                <strong>{round(entry.food.calories * entry.amount / 100)} kcal</strong>
                <span>P {round(entry.food.protein * entry.amount / 100, 1)} · K {round(entry.food.carbs * entry.amount / 100, 1)} · F {round(entry.food.fat * entry.amount / 100, 1)}</span>
              </div>
              <div className="entry-actions">
                <button className="edit-button" onClick={() => onEdit(entry)} aria-label="Eintrag bearbeiten"><PencilLine size={17} /></button>
                <button className="delete-button" onClick={() => onDelete(entry.id)} aria-label="Eintrag löschen"><Trash2 size={17} /></button>
              </div>
            </div>
          ))}
        </div>
      )}
    </article>
  )
}

function EditEntryDialog({ entry, onClose, onSaved }: {
  entry: Entry
  onClose: () => void
  onSaved: (meal: Meal, amount: number, quantity: number, unitAmount: number) => void
}) {
  const [meal, setMeal] = useState<Meal>(entry.meal)
  const [amount, setAmount] = useState(entry.amount)
  const unitAmount = entry.unitAmount || entry.food.servingSize || entry.amount
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const save = async (event: React.FormEvent) => {
    event.preventDefault()
    if (amount <= 0) return
    setBusy(true); setError('')
    try {
      const quantity = amount / unitAmount
      await api.updateEntry(entry.id, meal, amount, quantity, unitAmount)
      onSaved(meal, amount, quantity, unitAmount)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Eintrag konnte nicht gespeichert werden')
      setBusy(false)
    }
  }

  return (
    <div className="dialog-backdrop" onMouseDown={event => event.target === event.currentTarget && onClose()}>
      <form className="dialog edit-entry-dialog" onSubmit={save}>
        <div className="dialog-head">
          <div><span>Eintrag bearbeiten</span><h2>{entry.food.name}</h2></div>
          <button type="button" className="icon-button" onClick={onClose}><X /></button>
        </div>
        <div className="selected-title edit-entry-food">
          <div className="large-thumb">{entry.food.imageUrl ? <img src={entry.food.imageUrl} alt="" /> : entry.food.name[0]}</div>
          <div><h3>{entry.food.name}</h3><p>{entry.food.brand || 'Eigenes Lebensmittel'}</p></div>
        </div>
        <div className="field-grid edit-entry-fields">
          <label className="field"><span>Mahlzeit</span>
            <select value={meal} onChange={event => setMeal(event.target.value as Meal)}>
              {meals.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
            </select>
          </label>
          <label className="field"><span>Menge in Gramm</span>
            <input autoFocus type="number" min="0.1" max="10000" step="0.1"
              value={amount} onChange={event => setAmount(Number(event.target.value))} />
          </label>
        </div>
        <QuantityModifier amount={amount} unitAmount={unitAmount} onAmountChange={setAmount} />
        <div className="edit-entry-preview">
          {round(entry.food.calories * amount / 100)} kcal ·
          {' '}P {round(entry.food.protein * amount / 100, 1)} g ·
          {' '}K {round(entry.food.carbs * amount / 100, 1)} g ·
          {' '}F {round(entry.food.fat * amount / 100, 1)} g
        </div>
        {error && <div className="inline-error">{error}</div>}
        <button className="primary-button full" disabled={busy || amount <= 0}>
          {busy ? <LoaderCircle className="spin" /> : <PencilLine />} Änderungen speichern
        </button>
      </form>
    </div>
  )
}

function NutrientDetails({ totals }: { totals: Totals }) {
  const details = [
    ['Zucker', totals.sugar, 'g'],
    ['Gesättigte Fettsäuren', totals.saturatedFat, 'g'],
    ['Salz', totals.salt, 'g'],
    ...Object.entries(totals.micros).map(([name, value]) => [name, value, 'mg'] as [string, number, string]),
  ].filter(([, value]) => Number(value) > 0)

  return (
    <section className="details-card">
      <div className="section-heading compact"><div><span>Nährstoffdetails</span><h2>Makros & Mikros</h2></div></div>
      {details.length === 0
        ? <p className="muted">Mit deinen Einträgen erscheinen hier Zucker, Salz, Vitamine und Mineralstoffe.</p>
        : <div className="nutrient-grid">{details.map(([name, value, unit]) => (
          <div key={String(name)}><span>{name}</span><strong>{round(Number(value), 2)} {unit}</strong></div>
        ))}</div>}
    </section>
  )
}

function AddDialog({ date, meal, onClose, onAdded }: {
  date: string; meal: Meal; onClose: () => void; onAdded: () => void
}) {
  const [query, setQuery] = useState('')
  const [foods, setFoods] = useState<Food[]>([])
  const [selected, setSelected] = useState<Food | null>(null)
  const [amount, setAmount] = useState(100)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [manual, setManual] = useState(false)
  const [scanning, setScanning] = useState(false)
  const [editingServing, setEditingServing] = useState(false)
  const [servingDraft, setServingDraft] = useState(100)

  useEffect(() => {
    const timer = window.setTimeout(async () => {
      try { setFoods(await api.foods(query)) } catch { setFoods([]) }
    }, 180)
    return () => window.clearTimeout(timer)
  }, [query])

  const lookupBarcode = async (code = query) => {
    const clean = code.trim()
    if (!/^\d{8,14}$/.test(clean)) {
      setError('Bitte einen gültigen Barcode mit 8–14 Ziffern eingeben.')
      return
    }
    setBusy(true); setError('')
    try {
      const found = await api.barcode(clean)
      setSelected(found)
      setAmount(found.servingSize || 100)
      setServingDraft(found.servingSize || 100)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Barcode-Suche fehlgeschlagen')
    } finally { setBusy(false) }
  }

  const add = async () => {
    if (!selected || amount <= 0) return
    setBusy(true)
    try {
      let unitAmount = selected.servingSize || amount
      if (selected.needsServingSetup) {
        const updated = await api.updateServing(selected.id, amount)
        setSelected(updated)
        unitAmount = updated.servingSize
      }
      await api.createEntry(selected.id, date, meal, amount, amount / unitAmount, unitAmount)
      await onAdded()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Eintragen fehlgeschlagen')
      setBusy(false)
    }
  }

  const chooseFood = (food: Food) => {
    setSelected(food)
    setAmount(food.servingSize || 100)
    setServingDraft(food.servingSize || 100)
    setEditingServing(false)
  }

  const saveServing = async () => {
    if (!selected || servingDraft <= 0) return
    setBusy(true); setError('')
    try {
      const updated = await api.updateServing(selected.id, servingDraft)
      setSelected(updated)
      setAmount(updated.servingSize)
      setEditingServing(false)
      setFoods(current => current.map(food => food.id === updated.id ? updated : food))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Portionsgröße konnte nicht gespeichert werden')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="dialog-backdrop" onMouseDown={event => event.target === event.currentTarget && onClose()}>
      <div className="dialog">
        <div className="dialog-head">
          <div><span>Eintrag hinzufügen</span><h2>{meals.find(item => item.id === meal)?.label}</h2></div>
          <button className="icon-button" onClick={onClose}><X /></button>
        </div>
        {manual ? (
          <ManualFood onCancel={() => setManual(false)} onCreated={food => { chooseFood(food); setManual(false) }} />
        ) : selected ? (
          <div className="selected-food">
            <button className="back-link" onClick={() => setSelected(null)}><ChevronLeft size={17} /> Andere Auswahl</button>
            <div className="selected-title">
              <div className="large-thumb">{selected.imageUrl ? <img src={selected.imageUrl} alt="" /> : selected.name[0]}</div>
              <div><h3>{selected.name}</h3><p>{selected.brand || 'Eigenes Lebensmittel'}</p></div>
            </div>
            <div className="nutrition-preview">
              <span><strong>{round(selected.calories * amount / 100)}</strong> kcal</span>
              <span><strong>{round(selected.protein * amount / 100, 1)}</strong> g Protein</span>
              <span><strong>{round(selected.carbs * amount / 100, 1)}</strong> g KH</span>
              <span><strong>{round(selected.fat * amount / 100, 1)}</strong> g Fett</span>
            </div>
            <div className="portion-setting">
              <div className="portion-setting-head">
                <div>
                  <span>{selected.needsServingSetup ? 'Einmal festlegen' : 'Gespeicherte Standardmenge'}</span>
                  <strong>{selected.needsServingSetup
                    ? 'Diese Grammzahl wird beim Eintragen gespeichert'
                    : `${round(selected.servingSize, 1)} g`}</strong>
                </div>
                {!selected.needsServingSetup && (
                  <button onClick={() => { setServingDraft(selected.servingSize); setEditingServing(value => !value) }}>
                    <PencilLine size={15} /> Ändern
                  </button>
                )}
              </div>
              {editingServing && (
                <div className="serving-editor">
                  <label className="field"><span>Gramm pro Portion</span>
                    <input type="number" min="0.1" max="10000" step="0.1" value={servingDraft}
                      onChange={event => setServingDraft(Number(event.target.value))} />
                  </label>
                  <button className="primary-button" onClick={() => void saveServing()} disabled={busy}>Speichern</button>
                </div>
              )}
            </div>
            <label className="field"><span>{selected.needsServingSetup ? 'Standardmenge und heutige Menge in Gramm' : 'Menge in Gramm'}</span><div className="amount-input">
              <button onClick={() => setAmount(value => Math.max(1, value - 10))}><Minus /></button>
              <input type="number" min="1" max="10000" value={amount} onChange={e => setAmount(Number(e.target.value))} />
              <button onClick={() => setAmount(value => value + 10)}><Plus /></button>
            </div></label>
            <QuantityModifier amount={amount} unitAmount={selected.servingSize || amount} onAmountChange={setAmount} />
            <button className="primary-button full" onClick={add} disabled={busy}>
              {busy ? <LoaderCircle className="spin" /> : <Plus />} Eintragen
            </button>
          </div>
        ) : scanning ? (
          <Scanner onCode={code => { setScanning(false); setQuery(code); void lookupBarcode(code) }} onCancel={() => setScanning(false)} />
        ) : (
          <>
            <div className="search-box"><ScanLine /><input autoFocus placeholder="Lebensmittel oder Barcode suchen …" value={query} onChange={e => setQuery(e.target.value)} /></div>
            <div className="action-row">
              <button onClick={() => setScanning(true)}><Barcode /> Kamera scannen</button>
              <button onClick={() => void lookupBarcode()} disabled={busy}><ScanLine /> Barcode abrufen</button>
              <button onClick={() => setManual(true)}><CirclePlus /> Manuell anlegen</button>
            </div>
            {busy && <div className="loading-state small"><LoaderCircle className="spin" /> Produkt wird gesucht …</div>}
            <div className="food-results">
              {foods.map(food => (
                <button key={food.id} onClick={() => chooseFood(food)}>
                  <span className="food-thumb">{food.imageUrl ? <img src={food.imageUrl} alt="" /> : food.name[0]}</span>
                  <span><strong>{food.name}</strong><small>{food.brand || 'Manuell'} · {round(food.calories)} kcal / 100 g · Portion {round(food.servingSize, 1)} g</small></span>
                  <ChevronRight />
                </button>
              ))}
              {!busy && foods.length === 0 && <p className="muted center">Noch kein passendes Lebensmittel lokal gespeichert.</p>}
            </div>
          </>
        )}
        {error && <div className="inline-error">{error}</div>}
        <footer className="attribution">Produktdaten von <a href="https://world.openfoodfacts.org" target="_blank">Open Food Facts</a> (ODbL)</footer>
      </div>
    </div>
  )
}

function QuantityModifier({ amount, unitAmount, onAmountChange }: {
  amount: number
  unitAmount: number
  onAmountChange: (amount: number) => void
}) {
  const quantity = unitAmount > 0 ? amount / unitAmount : 1
  const setQuantity = (value: number) => onAmountChange(Math.max(1, value) * unitAmount)
  return (
    <div className="quantity-modifier">
      <div>
        <span>Anzahl</span>
        <small>je {round(unitAmount, 1)} g</small>
      </div>
      <div className="quantity-input">
        <button type="button" onClick={() => setQuantity(Math.max(1, Math.round(quantity) - 1))}><Minus size={16} /></button>
        <input type="number" min="0.01" step="0.01" value={round(quantity, 2)}
          onChange={event => setQuantity(Number(event.target.value))} />
        <button type="button" onClick={() => setQuantity(Math.max(1, Math.round(quantity) + 1))}><Plus size={16} /></button>
      </div>
      <strong>{round(amount, 1)} g gesamt</strong>
    </div>
  )
}

function Scanner({ onCode, onCancel }: { onCode: (code: string) => void; onCancel: () => void }) {
  const [message, setMessage] = useState('Kamera wird gestartet …')

  useEffect(() => {
    let stream: MediaStream | undefined
    let timer = 0
    let stopped = false
    const video = document.querySelector<HTMLVideoElement>('#scanner-video')

    async function start() {
      if (!video || !('BarcodeDetector' in window)) {
        setMessage('Dein Browser unterstützt den Kamera-Scanner nicht. Tippe den Barcode bitte ein.')
        return
      }
      try {
        stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' }, audio: false })
        video.srcObject = stream
        await video.play()
        const Detector = (window as unknown as { BarcodeDetector: new (options: { formats: string[] }) => { detect: (source: HTMLVideoElement) => Promise<{ rawValue: string }[]> } }).BarcodeDetector
        const detector = new Detector({ formats: ['ean_13', 'ean_8', 'upc_a', 'upc_e'] })
        setMessage('Barcode in den Rahmen halten')
        const detect = async () => {
          if (stopped) return
          const codes = await detector.detect(video).catch(() => [])
          if (codes[0]?.rawValue) onCode(codes[0].rawValue)
          else timer = window.setTimeout(detect, 250)
        }
        void detect()
      } catch {
        setMessage('Kamera konnte nicht geöffnet werden. Prüfe Berechtigung und HTTPS.')
      }
    }
    void start()
    return () => {
      stopped = true
      window.clearTimeout(timer)
      stream?.getTracks().forEach(track => track.stop())
    }
  }, [onCode])

  return (
    <div className="scanner">
      <video id="scanner-video" muted playsInline />
      <div className="scan-frame" />
      <p>{message}</p>
      <button className="secondary-button" onClick={onCancel}>Abbrechen</button>
    </div>
  )
}

function ManualFood({ onCancel, onCreated }: { onCancel: () => void; onCreated: (food: Food) => void }) {
  const [form, setForm] = useState({
    name: '', brand: '', barcode: '', calories: 0, protein: 0, carbs: 0,
    fat: 0, fiber: 0, sugar: 0, saturatedFat: 0, salt: 0, servingSize: 100,
  })
  const [micros, setMicros] = useState<Record<string, number>>({
    Calcium: 0, Eisen: 0, Magnesium: 0, Kalium: 0, Zink: 0,
    'Vitamin C': 0, 'Vitamin B12': 0, 'Vitamin D': 0,
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const set = (key: keyof typeof form, value: string) =>
    setForm(current => ({ ...current, [key]: key === 'name' || key === 'brand' || key === 'barcode' ? value : Number(value) }))

  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setBusy(true)
    try {
      const food = await api.createFood({
        ...form,
        servingUnit: 'g',
        micros: Object.fromEntries(Object.entries(micros).filter(([, value]) => value > 0)),
      })
      onCreated(food)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Speichern fehlgeschlagen')
      setBusy(false)
    }
  }

  return (
    <form className="manual-form" onSubmit={submit}>
      <button type="button" className="back-link" onClick={onCancel}><ChevronLeft size={17} /> Zurück zur Suche</button>
      <p className="form-hint">Alle Nährwerte beziehen sich auf 100 g.</p>
      <div className="field-grid">
        <label className="field wide"><span>Name *</span><input required value={form.name} onChange={e => set('name', e.target.value)} /></label>
        <label className="field"><span>Marke</span><input value={form.brand} onChange={e => set('brand', e.target.value)} /></label>
        <label className="field"><span>Barcode</span><input inputMode="numeric" value={form.barcode} onChange={e => set('barcode', e.target.value)} /></label>
        <label className="field wide"><span>Standardmenge beim Eintragen (g)</span>
          <input type="number" min="0.1" max="10000" step="0.1" value={form.servingSize}
            onChange={e => set('servingSize', e.target.value)} />
        </label>
        {([
          ['calories', 'Kalorien', 'kcal'], ['protein', 'Protein', 'g'], ['carbs', 'Kohlenhydrate', 'g'],
          ['fat', 'Fett', 'g'], ['fiber', 'Ballaststoffe', 'g'], ['sugar', 'Zucker', 'g'],
          ['saturatedFat', 'Gesättigt', 'g'], ['salt', 'Salz', 'g'],
        ] as const).map(([key, label, unit]) => (
          <label className="field" key={key}><span>{label} ({unit})</span><input type="number" min="0" step="0.01" value={form[key]} onChange={e => set(key, e.target.value)} /></label>
        ))}
        <div className="form-divider wide">Mikronährstoffe (mg pro 100 g)</div>
        {Object.entries(micros).map(([name, value]) => (
          <label className="field" key={name}>
            <span>{name} (mg)</span>
            <input type="number" min="0" step="0.001" value={value}
              onChange={event => setMicros(current => ({ ...current, [name]: Number(event.target.value) }))} />
          </label>
        ))}
      </div>
      {error && <div className="inline-error">{error}</div>}
      <button className="primary-button full" disabled={busy}>{busy ? <LoaderCircle className="spin" /> : <Plus />} Speichern und auswählen</button>
    </form>
  )
}

function GoalsDialog({ goals, onClose, onSave }: { goals: Goals; onClose: () => void; onSave: (goals: Goals) => void }) {
  const [form, setForm] = useState(goals)
  const [busy, setBusy] = useState(false)
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setBusy(true)
    try { onSave(await api.updateGoals(form)) } finally { setBusy(false) }
  }
  return (
    <div className="dialog-backdrop" onMouseDown={event => event.target === event.currentTarget && onClose()}>
      <form className="dialog goals-dialog" onSubmit={submit}>
        <div className="dialog-head"><div><span>Einstellungen</span><h2>Deine Tagesziele</h2></div><button type="button" className="icon-button" onClick={onClose}><X /></button></div>
        <div className="field-grid">
          {Object.entries(form).map(([key, value]) => (
            <label className="field" key={key}><span>{({ calories: 'Kalorien (kcal)', protein: 'Protein (g)', carbs: 'Kohlenhydrate (g)', fat: 'Fett (g)', fiber: 'Ballaststoffe (g)' } as Record<string, string>)[key]}</span>
              <input type="number" min="0" value={value} onChange={e => setForm(current => ({ ...current, [key]: Number(e.target.value) }))} />
            </label>
          ))}
        </div>
        <button className="primary-button full" disabled={busy}>Ziele speichern</button>
      </form>
    </div>
  )
}
