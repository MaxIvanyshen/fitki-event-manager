package service

import "giveaway-tool/database/sqlc"

const errHTML = `
<div class="bg-red-50 border-l-4 border-red-500 p-4" id="error">
    <div class="flex">
        <div class="flex-shrink-0">
            <svg class="h-5 w-5 text-red-500" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
            </svg>
        </div>
        <div class="ml-3">
            <p class="text-sm text-red-700">%s</p>
        </div>
    </div>
</div>
`

const successHTML = `
<div class="bg-green-50 border-l-4 border-green-500 p-4" id="success">
    <div class="flex">
        <div class="flex-shrink-0">
            <svg class="h-5 w-5 text-green-500" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
            </svg>
        </div>
        <div class="ml-3">
            <p class="text-sm text-green-700">%s</p>
        </div>
    </div>
</div>
`

const votesForm = `
	<form hx-patch="/admin/events/{{ $.Event.ID }}/users/{{ .ID }}" 
		  hx-target="closest td" 
		  hx-swap="innerHTML"
		  class="flex items-center space-x-2">
		<input type="number" name="n" value="%d" min="1"
			   class="w-16 py-1 px-2 text-sm border border-gray-300 rounded focus:border-indigo-500 focus:ring-indigo-500">
		<button type="submit" class="text-indigo-600 hover:text-indigo-900">
			<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
			</svg>
		</button>
	</form>
`

type AdminData struct {
	Username string
	Password string
}

type Data struct {
	Events         []*sqlc.Events `json:"events"`
	CurrentEventID int64          `json:"current_event_id"`
	IsAdmin        bool           `json:"isAdmin"`
}
