export namespace packs {
	
	export class PackSummary {
	    id: string;
	    displayName: string;
	    description: string;
	    isProtectedByAuthor: boolean;
	    isExperimental: boolean;
	    modCount: number;
	
	    static createFrom(source: any = {}) {
	        return new PackSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.displayName = source["displayName"];
	        this.description = source["description"];
	        this.isProtectedByAuthor = source["isProtectedByAuthor"];
	        this.isExperimental = source["isExperimental"];
	        this.modCount = source["modCount"];
	    }
	}

}

export namespace profiles {
	
	export class Summary {
	    id: string;
	    name: string;
	    path: string;
	    game_version: string;
	    loader: string;
	    loader_version?: string;
	    group?: string;
	    selected_pixelplay_pack_id?: string;
	    state: string;
	    description?: string;
	    is_standard_version: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.game_version = source["game_version"];
	        this.loader = source["loader"];
	        this.loader_version = source["loader_version"];
	        this.group = source["group"];
	        this.selected_pixelplay_pack_id = source["selected_pixelplay_pack_id"];
	        this.state = source["state"];
	        this.description = source["description"];
	        this.is_standard_version = source["is_standard_version"];
	    }
	}

}

