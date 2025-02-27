<template>
	<div class="container px-4">
		<header class="d-flex justify-content-between align-items-center py-3">
			<h1 class="mb-1 pt-1 d-flex text-nowrap">
				<router-link class="dropdown-item mx-2 me-2" to="/">
					<shopicon-bold-arrowback size="s" class="back"></shopicon-bold-arrowback>
				</router-link>
				{{ $t("sessions.title") }}
			</h1>
			<TopNavigation />
		</header>

		<div class="row">
			<main class="col-12">
				<div class="mb-4">
					<a
						class="btn btn-outline-secondary text-nowrap my-2"
						:href="`./api/sessions?format=csv&amp;lang=${$i18n.locale}`"
						download="sessions.csv"
					>
						{{ $t("sessions.downloadCsv") }}
					</a>
				</div>

				<div v-for="group in sessionsByMonthAndLoadpoint" :key="group.month">
					<div class="mx-2">
						<div class="d-flex align-items-baseline my-5">
							<h2 class="me-4 mb-0">
								{{ formatGroupHeadline(group.month) }}
							</h2>
						</div>

						<div v-for="loadpoint in group.loadpoints" :key="loadpoint.name">
							<div class="d-flex align-items-baseline mb-3">
								<h3 class="me-4 mb-0">
									{{ loadpoint.name }}
								</h3>
								<div class="large">{{ fmtKWh(loadpoint.total) }}</div>
							</div>

							<ul class="breakdown text-gray d-sm-flex flex-sm-wrap ps-0 mb-2">
								<li
									v-for="(vehicle, id) in groupedKWh(
										'vehicle',
										loadpoint.sessions
									)"
									:key="id"
									class="breakdown-item"
								>
									{{ vehicle.name }}: {{ fmtKWh(vehicle.energy) }}
								</li>
							</ul>
							<div class="table-responsive my-3">
								<table class="table">
									<thead>
										<tr>
											<th scope="col">{{ $t("sessions.vehicle") }}</th>
											<th scope="col" class="text-end ps-sm-4 pe-md-5">
												{{ $t("sessions.energy") }}
											</th>
											<th scope="col" class="ps-3 ps-md-4 ps-md-5">
												{{ $t("sessions.date") }}
											</th>
										</tr>
									</thead>
									<tbody>
										<tr
											v-for="(session, id) in loadpoint.sessions"
											:key="id"
											role="button"
											@click="showDetails(session)"
										>
											<td class="align-middle">
												{{ session.vehicle }}
											</td>
											<td
												class="text-nowrap text-end ps-sm-4 pe-md-5 align-middle"
											>
												{{ fmtKWh(session.chargedEnergy * 1e3) }}
											</td>
											<td class="text-nowrap ps-3 ps-md-4 ps-md-5">
												<span class="d-block d-sm-none">
													{{
														fmtFullDateTime(
															new Date(session.created),
															true
														)
													}}
													<br />
													{{
														fmtFullDateTime(
															new Date(session.finished),
															true
														)
													}}
												</span>
												<span class="d-none d-sm-block">
													{{
														fmtFullDateTime(
															new Date(session.created),
															false
														)
													}}
													<br />
													{{
														fmtFullDateTime(
															new Date(session.finished),
															false
														)
													}}
												</span>
											</td>
										</tr>
									</tbody>
								</table>
							</div>
						</div>
					</div>
				</div>
			</main>
			<ChargingSessionModal :session="selectedSession" @session-deleted="loadSessions" />
		</div>
	</div>
</template>

<script>
import Modal from "bootstrap/js/dist/modal";
import TopNavigation from "../components/TopNavigation.vue";
import "@h2d2/shopicons/es/bold/arrowback";
import "@h2d2/shopicons/es/regular/trash";
import formatter from "../mixins/formatter";
import api from "../api";
import ChargingSessionModal from "../components/ChargingSessionModal.vue";

export default {
	name: "ChargingSessions",
	components: { TopNavigation, ChargingSessionModal },
	mixins: [formatter],
	props: {
		notifications: Array,
	},
	data() {
		return { sessions: [], selectedSession: undefined };
	},
	computed: {
		sessionsByMonthAndLoadpoint() {
			const sessionsWithDefaults = this.sessions.map((session) => {
				const loadpoint = session.loadpoint || this.$t("main.loadpoint.fallbackName");
				const vehicle = session.vehicle || this.$t("main.vehicle.unknown");
				return { ...session, loadpoint, vehicle };
			});

			const sessionsByMonth = this.groupByMonth(sessionsWithDefaults);

			return Object.entries(sessionsByMonth).map(([month, sessions]) => {
				const loadpoints = Object.entries(this.groupByLoadpoint(sessions)).map(
					([loadpoint, sessionsByLoadpoint]) => {
						const total = this.totalKWh(sessionsByLoadpoint);
						return { name: loadpoint, total, sessions: sessionsByLoadpoint };
					}
				);
				return { month, loadpoints };
			});
		},
	},
	mounted() {
		this.loadSessions();
	},
	methods: {
		async loadSessions() {
			const response = await api.get("sessions");
			this.sessions = response.data?.result;
		},
		groupByMonth(sessions) {
			return sessions.reduce((groups, session) => {
				const date = new Date(session.finished);
				const month = `${date.getFullYear()}.${date.getMonth()}`;
				if (!groups[month]) groups[month] = [];
				groups[month].push(session);
				return groups;
			}, {});
		},
		groupByLoadpoint(sessions) {
			return sessions.reduce((groups, session) => {
				const loadpoint = session.loadpoint;
				if (!groups[loadpoint]) groups[loadpoint] = [];
				groups[loadpoint].push(session);
				return groups;
			}, {});
		},
		totalKWh(sessions) {
			return sessions.reduce((total, session) => total + session.chargedEnergy, 0) * 1e3;
		},
		groupedKWh(by, sessions) {
			const grouped = sessions.reduce((groups, session) => {
				const name = session[by];
				if (!groups[name]) groups[name] = 0;
				groups[name] += session.chargedEnergy * 1e3;
				return groups;
			}, {});
			const list = Object.entries(grouped).map(([name, energy]) => {
				return { name, energy };
			});
			return list.length >= 2 ? list : [];
		},
		formatGroupHeadline(group) {
			const date = new Date();
			const [year, month] = group.split(".");
			date.setMonth(month);
			date.setFullYear(year);
			return this.fmtMonthYear(date);
		},
		showDetails(session) {
			this.selectedSession = session;
			const modal = Modal.getOrCreateInstance(document.getElementById("sessionDetailsModal"));
			modal.show();
		},
	},
};
</script>
<style scoped>
.back {
	width: 22px;
	height: 22px;
	position: relative;
	top: -2px;
}
.breakdown {
	list-style: none;
}

.breakdown-item {
	white-space: nowrap;
}

/* breakpoint sm */
@media (min-width: 576px) {
	.breakdown-item:after {
		content: ", ";
		white-space: wrap;
		margin-right: 0.25rem;
	}
	.breakdown-item:last-child:after {
		content: "";
	}
}

.breakdown:empty {
	display: none;
}
</style>
