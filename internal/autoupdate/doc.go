package autoupdate

// TODO: where to put this package level documentary?

/* The autoupdater responds to the following github Events.
   Status-Event:
	A status Event is associated with one or more commits.
	For all branches that contain the commit and are associated to a
	pull-request for that have autoupdates are suspended goordinator
	retrieves the combined status for the pull-request.
	If the combined status is successful and updates for the pull-request
	are currently suspended, updates are resumed.
    Push Event:
        When a push-event is received and it was for:
	    - a base branch for an monitored pull-request, the first PR in the
	      queue for the base-branch is updated,

   Pull-Request Event:
       synchronize action:
	   When a synchronize Pull-Request event is received and the event was
	   for monitored pull-request and updates for it are currently
	   suspendend, updates will be resumed.

When the label and automerge trigger is enabled, it reacts both on them.
If a PR has a trigger and automerge-enabled and one of them is disabled, it is
removed from the queue, despite the other trigger is still active.

*/
