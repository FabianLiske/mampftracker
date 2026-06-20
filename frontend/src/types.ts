export type Micros = Record<string, number>

export interface Food {
  id: number
  name: string
  brand: string
  barcode: string
  servingSize: number
  servingUnit: string
  calories: number
  protein: number
  carbs: number
  fat: number
  fiber: number
  sugar: number
  saturatedFat: number
  salt: number
  micros: Micros
  source: 'manual' | 'openfoodfacts'
  imageUrl: string
  needsServingSetup?: boolean
}

export type Meal = 'breakfast' | 'lunch' | 'dinner' | 'snack' | 'drinks'

export interface Entry {
  id: number
  foodId: number
  date: string
  meal: Meal
  amount: number
  quantity: number
  unitAmount: number
  food: Food
  createdAt: string
}

export interface Goals {
  calories: number
  protein: number
  carbs: number
  fat: number
  fiber: number
}

export interface DailyStats {
  date: string
  weight: number | null
  caloriesBurned: number | null
}

export interface Totals extends Goals {
  sugar: number
  saturatedFat: number
  salt: number
  micros: Micros
}
